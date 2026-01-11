from __future__ import annotations

import argparse
import io
import json
import hashlib
import logging
from pathlib import Path
from typing import Dict, Tuple, List
import zipfile

import numpy as np
import pandas as pd

import torch
import torch.nn as nn
from torch.utils.data import TensorDataset, DataLoader

from sklearn.model_selection import train_test_split
from sklearn.preprocessing import StandardScaler
from sklearn.metrics import classification_report, confusion_matrix, accuracy_score, roc_auc_score

from sklearn.utils.class_weight import compute_class_weight

from model.grpc.grpc_client import send_model_to_peer

# -------------------------
# Logging
# -------------------------
def setup_logging(log_dir: Path, run_name: str) -> logging.Logger:
    log_dir.mkdir(parents=True, exist_ok=True)
    logger = logging.getLogger("diabetes_train")
    logger.setLevel(logging.INFO)
    logger.handlers.clear()

    fmt = logging.Formatter(
        fmt="%(asctime)s | %(levelname)s | %(message)s",
        datefmt="%Y-%m-%d %H:%M:%S",
    )

    ch = logging.StreamHandler()
    ch.setLevel(logging.INFO)
    ch.setFormatter(fmt)
    logger.addHandler(ch)

    fh = logging.FileHandler(log_dir / f"{run_name}.log", encoding="utf-8")
    fh.setLevel(logging.INFO)
    fh.setFormatter(fmt)
    logger.addHandler(fh)

    return logger


# -------------------------
# Data utilities
# -------------------------
def to_tensor_dataset(X: pd.DataFrame, y: pd.DataFrame) -> TensorDataset:
    X_tensor = torch.tensor(X.values, dtype=torch.float32)
    y_tensor = torch.tensor(y.values.flatten(), dtype=torch.long)
    return TensorDataset(X_tensor, y_tensor)


# -------------------------
# Model
# -------------------------
class MLP(nn.Module):
    def __init__(self, input_dim: int):
        super().__init__()
        self.net = nn.Sequential(
            nn.Linear(input_dim, 256),
            nn.BatchNorm1d(256),
            nn.ReLU(),
            nn.Dropout(0.3),

            nn.Linear(256, 128),
            nn.BatchNorm1d(128),
            nn.ReLU(),
            nn.Dropout(0.3),

            nn.Linear(128, 2),
        )

    def forward(self, x: torch.Tensor) -> torch.Tensor:
        return self.net(x)


@torch.no_grad()
def predict_proba(model: nn.Module, dl: DataLoader, device: torch.device) -> Tuple[np.ndarray, np.ndarray]:
    model.eval()
    all_probs: List[np.ndarray] = []
    all_y: List[np.ndarray] = []
    softmax = nn.Softmax(dim=1)
    for xb, yb in dl:
        xb = xb.to(device)
        logits = model(xb)
        probs = softmax(logits)[:, 1].detach().cpu().numpy()
        all_probs.append(probs)
        all_y.append(yb.numpy())
    return np.concatenate(all_probs), np.concatenate(all_y)


def weight_stats(state_dict: Dict[str, torch.Tensor]) -> Dict[str, Dict[str, float]]:
    """
    Lightweight per-tensor statistics that are cheap to compute and useful for debugging.
    """
    stats: Dict[str, Dict[str, float]] = {}
    for k, v in state_dict.items():
        t = v.detach().float().cpu()
        stats[k] = {
            "mean": float(t.mean().item()),
            "std": float(t.std(unbiased=False).item()),
            "l2": float(torch.linalg.vector_norm(t).item()),
            "min": float(t.min().item()),
            "max": float(t.max().item()),
        }
    return stats

def export_state_dict_binary(
    state_dict: Dict[str, torch.Tensor],
    model_id: str,
    epoch: int,
    num_samples: int
) -> bytes:
    """
    Export a PyTorch state_dict to an in-memory zip containing
    .weights.bin and .meta.json. Returns the zip as bytes.

    Args:
        state_dict: model.state_dict()
        model_id: unique model identifier
        epoch: current epoch number
        num_samples: number of samples used in training

    Returns:
        zip_bytes: bytes of zipped weights and meta
    """
    tensors_meta = []
    offset = 0
    all_bytes = bytearray()

    # Flatten all tensors into a single bytes array and record metadata
    for name, tensor in state_dict.items():
        t = tensor.detach().cpu().float().numpy()
        raw = t.tobytes()

        tensors_meta.append({
            "name": name,
            "dtype": "float32",
            "shape": list(t.shape),
            "offset": offset,
            "size": len(raw)
        })

        all_bytes.extend(raw)
        offset += len(raw)

    # Architecture hash (optional, ensures consistent model)
    arch_hash = hashlib.sha256(
        json.dumps([(m["name"], m["shape"]) for m in tensors_meta]).encode()
    ).hexdigest()

    meta = {
        "model_id": model_id,
        "arch_hash": arch_hash,
        "epoch": epoch,
        "num_samples": num_samples,
        "tensors": tensors_meta
    }

    # Create in-memory zip
    buffer = io.BytesIO()
    with zipfile.ZipFile(buffer, mode="w", compression=zipfile.ZIP_DEFLATED) as zf:
        zf.writestr(f"{model_id}.weights.bin", all_bytes)
        zf.writestr(f"{model_id}.meta.json", json.dumps(meta, indent=2))
    buffer.seek(0)

    return buffer.read()

def save_state_dict(
    model: nn.Module,
    out_dir: Path,
    epoch: int,
    save_json: bool,
    logger: logging.Logger,
) -> Tuple[Path, Path | None]:
    out_dir.mkdir(parents=True, exist_ok=True)
    pt_path = out_dir / f"mlp_epoch_{epoch:03d}.pt"
    torch.save(model.state_dict(), pt_path)
    json_path: Path | None = None

    if save_json:
        state = {k: v.detach().cpu().tolist() for k, v in model.state_dict().items()}
        json_path = out_dir / f"mlp_epoch_{epoch:03d}.json"
        with open(json_path, "w", encoding="utf-8") as f:
            json.dump(state, f)

    logger.info(f"Saved checkpoint: {pt_path}" + (f" and {json_path}" if json_path else ""))
    return pt_path, json_path


def main() -> int:
    ap = argparse.ArgumentParser(description="Train a single diabetes MLP and log weights each epoch.")
    ap.add_argument("--epochs", type=int, default=10)
    ap.add_argument("--batch-size", type=int, default=1024)
    ap.add_argument("--lr", type=float, default=1e-3)
    ap.add_argument("--weight-decay", type=float, default=1e-4)
    ap.add_argument("--seed", type=int, default=42)
    ap.add_argument("--threshold", type=float, default=0.52, help="Decision threshold for class 1.")
    ap.add_argument("--out-dir", type=str, default="runs/diabetes_mlp", help="Base output directory.")
    ap.add_argument("--run-name", type=str, default=None, help="Run name (defaults to timestamped name).")
    ap.add_argument("--save-json", action="store_true", help="Also export weights to JSON each epoch (very large).")
    ap.add_argument("--log-weight-stats", action="store_true", help="Log per-tensor weight stats each epoch.")
    ap.add_argument("--save-weight-stats-json", action="store_true",
                    help="Save per-tensor weight stats JSON per epoch (small).")
    ap.add_argument("--device", type=str, default=None, help="Force device: cpu or cuda. Default: auto.")
    args = ap.parse_args()

    # Reproducibility
    np.random.seed(args.seed)
    torch.manual_seed(args.seed)
    torch.cuda.manual_seed_all(args.seed)

    out_dir = Path(args.out_dir)
    ckpt_dir = out_dir / "checkpoints"
    log_dir = out_dir / "logs"
    stats_dir = out_dir / "weight_stats"

    if args.run_name is None:
        from datetime import datetime
        args.run_name = f"run_{datetime.now().strftime('%Y%m%d_%H%M%S')}"
    logger = setup_logging(log_dir, args.run_name)

    device = torch.device(args.device) if args.device else torch.device("cuda" if torch.cuda.is_available() else "cpu")
    logger.info(f"Using device: {device}")

    # Fetch dataset
    logger.info("Loading dataset (UCI id=891) from local CSV...")

    data_path = "data/diabetes_binary_health_indicators_BRFSS2015.csv"
    df = pd.read_csv(data_path)

    X: pd.DataFrame = df.drop(columns=["Diabetes_binary"]).copy()
    y: pd.DataFrame = df[["Diabetes_binary"]].copy()

    # Prep data (from notebook)
    continuous_cols = ["BMI", "Age", "Income"]
    scaler = StandardScaler()
    X[continuous_cols] = scaler.fit_transform(X[continuous_cols])

    X_train, X_temp, y_train, y_temp = train_test_split(
        X, y, test_size=0.3, stratify=y, random_state=args.seed
    )
    X_val, X_test, y_val, y_test = train_test_split(
        X_temp, y_temp, test_size=0.5, stratify=y_temp, random_state=args.seed
    )
    logger.info(f"Splits: Train {X_train.shape}, Val {X_val.shape}, Test {X_test.shape}")

    # Datasets / loaders
    train_ds = to_tensor_dataset(X_train, y_train)
    val_ds = to_tensor_dataset(X_val, y_val)
    test_ds = to_tensor_dataset(X_test, y_test)

    pin = (device.type == "cuda")
    train_dl = DataLoader(train_ds, batch_size=args.batch_size, shuffle=True, num_workers=0, pin_memory=pin)
    val_dl = DataLoader(val_ds, batch_size=args.batch_size, shuffle=False, num_workers=0, pin_memory=pin)
    test_dl = DataLoader(test_ds, batch_size=args.batch_size, shuffle=False, num_workers=0, pin_memory=pin)

    # Class weights (from notebook)
    class_weights = compute_class_weight(
        class_weight="balanced",
        classes=np.unique(y_train.values.flatten()),
        y=y_train.values.flatten(),
    )
    class_weights_t = torch.tensor(class_weights, dtype=torch.float32).to(device)
    logger.info(f"Class weights: {class_weights.tolist()}")

    # Model, loss, optimizer
    model = MLP(input_dim=X_train.shape[1]).to(device)
    criterion = nn.CrossEntropyLoss(weight=class_weights_t)
    optimizer = torch.optim.AdamW(model.parameters(), lr=args.lr, weight_decay=args.weight_decay)

    # Scheduler (as in notebook)
    scheduler = torch.optim.lr_scheduler.ReduceLROnPlateau(
        optimizer, mode="min", factor=0.5, patience=2
    )

    def run_epoch(dl: DataLoader, train: bool) -> float:
        if train:
            model.train()
        else:
            model.eval()
        running = 0.0
        n = 0

        for xb, yb in dl:
            xb = xb.to(device, non_blocking=True)
            yb = yb.to(device, non_blocking=True)

            if train:
                optimizer.zero_grad(set_to_none=True)

            logits = model(xb)
            loss = criterion(logits, yb)

            if train:
                loss.backward()
                optimizer.step()

            bs = xb.size(0)
            running += float(loss.item()) * bs
            n += bs

        return running / max(n, 1)

    best_val = float("inf")
    best_state: Dict[str, torch.Tensor] | None = None

    history: List[Dict[str, float]] = []

    logger.info("Starting training...")
    for epoch in range(1, args.epochs + 1):
        tr_loss = run_epoch(train_dl, train=True)
        val_loss = run_epoch(val_dl, train=False)

        scheduler.step(val_loss)

        lr_now = float(optimizer.param_groups[0]["lr"])
        logger.info(f"Epoch {epoch:03d}/{args.epochs} | train_loss={tr_loss:.6f} val_loss={val_loss:.6f} lr={lr_now:.2e}")

        # Save weights each epoch
        save_state_dict(model, ckpt_dir, epoch, args.save_json, logger)

        # Send to server
        zip_bytes = export_state_dict_binary(
            state_dict=model.state_dict(),
            model_id=f"{args.run_name}_epoch_{epoch:03d}",
            epoch=epoch,
            num_samples=len(train_ds)
        )

        msg = send_model_to_peer(
            model_id=f"{args.run_name}_epoch_{epoch:03d}",
            content_bytes=zip_bytes
        )

        # Optionally log/save weight stats
        if args.log_weight_stats or args.save_weight_stats_json:
            stats = weight_stats(model.state_dict())
            if args.log_weight_stats:
                # Log top-level summary only (full stats are too verbose)
                # We'll log L2 norms for the main trainable tensors.
                l2s = {k: v["l2"] for k, v in stats.items() if k.endswith("weight")}
                # Keep logs readable
                top = dict(list(sorted(l2s.items(), key=lambda kv: kv[0]))[:8])
                logger.info(f"Weight L2 norms (sample): {top}")
            if args.save_weight_stats_json:
                stats_dir.mkdir(parents=True, exist_ok=True)
                with open(stats_dir / f"weight_stats_epoch_{epoch:03d}.json", "w", encoding="utf-8") as f:
                    json.dump(stats, f)

        history.append({"epoch": epoch, "train_loss": tr_loss, "val_loss": val_loss, "lr": lr_now})

        # Best checkpoint in-memory
        if val_loss < best_val - 1e-6:
            best_val = val_loss
            best_state = {k: v.detach().cpu().clone() for k, v in model.state_dict().items()}
            logger.info(f"New best val loss: {best_val:.6f}")

    # Load best weights for evaluation (like notebook)
    if best_state is not None:
        model.load_state_dict({k: v.to(device) for k, v in best_state.items()})

    # Save training history
    out_dir.mkdir(parents=True, exist_ok=True)
    with open(out_dir / "history.json", "w", encoding="utf-8") as f:
        json.dump(history, f, indent=2)
    logger.info(f"Saved training history: {out_dir / 'history.json'}")

    # Evaluate
    y_proba_test, y_true_test = predict_proba(model, test_dl, device=device)
    y_pred_test = (y_proba_test >= args.threshold).astype(int)

    acc = accuracy_score(y_true_test, y_pred_test)
    auc_ = roc_auc_score(y_true_test, y_proba_test)

    logger.info("=== Test Evaluation (imbalanced) ===")
    logger.info(f"Accuracy: {acc:.6f}")
    logger.info(f"ROC-AUC:   {auc_:.6f}")
    logger.info("Classification report:\n" + classification_report(y_true_test, y_pred_test))
    logger.info("Confusion matrix:\n" + str(confusion_matrix(y_true_test, y_pred_test)))

    # Also dump metrics to a json file
    metrics = {"accuracy": float(acc), "roc_auc": float(auc_), "threshold": float(args.threshold)}
    with open(out_dir / "metrics.json", "w", encoding="utf-8") as f:
        json.dump(metrics, f, indent=2)
    logger.info(f"Saved metrics: {out_dir / 'metrics.json'}")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
