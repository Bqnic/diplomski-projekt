from __future__ import annotations

import argparse
import io
import os
import json
import logging
from pathlib import Path
from typing import Dict, Tuple, List

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

def export_model_buffer(
    model: torch.nn.Module,
    epoch: int,
    num_samples: int,
    model_id: str
) -> Tuple[io.BytesIO, str]:
    """
    Serialize the model into a single buffer (like .pt), ready to be sent over gRPC.

    Returns:
        buffer: BytesIO containing the serialized model
        model_id: unique identifier for this epoch/model
    """
    buffer = io.BytesIO()
    
    ckpt = {
        "model_id": model_id,
        "epoch": epoch,
        "num_samples": num_samples,
        "state_dict": {k: v.cpu() for k, v in model.state_dict().items()},
    }

    torch.save(ckpt, buffer)
    buffer.seek(0)
    
    return buffer, model_id

def split_data(X, y, NODE_NAME: str, NUM_NODES: int):
    id = int(NODE_NAME.split("-")[-1])

    indices = np.arange(len(X))
    split_indices = indices[id::NUM_NODES]

    return X.iloc[split_indices], y.iloc[split_indices]

def main() -> int:
    NODE_NAME = os.environ.get("NODE_NAME")
    NUM_NODES = int(os.environ.get("NUM_NODES"))
    SENDING_EPOCH = int(os.environ.get("SENDING_EPOCH"))
    AGGREGATING_EPOCH = int(os.environ.get("AGGREGATING_EPOCH"))

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
    log_dir = out_dir / "logs"
    stats_dir = out_dir / "weight_stats"

    if args.run_name is None:
        from datetime import datetime
        args.run_name = f"run_{datetime.now().strftime('%Y%m%d_%H%M%S')}"
    logger = setup_logging(log_dir, args.run_name)
    logger.info(f"Node name: {NODE_NAME}")
    logger.info(f"Number of nodes: {NUM_NODES}")

    device = torch.device(args.device) if args.device else torch.device("cuda" if torch.cuda.is_available() else "cpu")
    logger.info(f"Using device: {device}")

    # Fetch dataset
    logger.info("Loading dataset (UCI id=891) from local CSV...")

    data_path = "data/diabetes_binary_health_indicators_BRFSS2015.csv"
    df = pd.read_csv(data_path)

    X_temp: pd.DataFrame = df.drop(columns=["Diabetes_binary"]).copy()
    y_temp: pd.DataFrame = df[["Diabetes_binary"]].copy()

    X, y = split_data(X_temp.copy(), y_temp.copy(), NODE_NAME, NUM_NODES)
    logger.info(f"Taking every example starting from {NODE_NAME}, step {NUM_NODES}")

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

        if epoch % SENDING_EPOCH == 0:
            # Send to server
            buffer, mid = export_model_buffer(model, epoch, len(train_ds), model_id=f"{NODE_NAME}_epoch_{epoch:03d}")
            msg = send_model_to_peer(buffer, mid)
            logger.info(f"Sent model to peer: {msg}")

        if epoch % AGGREGATING_EPOCH == 0:
            # -------------------------
            # Aggregate remote + local models
            # - include local model
            # - epoch-weighted remote averaging
            # - alpha = 0.75
            # - load ALL files (torch.save produces zip-like blobs)
            # -------------------------
            AGG_ALPHA = 0.75

            remote_dir = Path("/app/shared/remote-models")
            aggregated_dir = Path("/app/shared/aggregated")
            aggregated_dir.mkdir(parents=True, exist_ok=True)

            if remote_dir.exists():
                remote_files = [p for p in remote_dir.iterdir() if p.is_file()]

                if remote_files:
                    logger.info(f"Aggregating {len(remote_files)} remote model blobs")

                    # ---- Local model
                    local_state = {
                        k: v.detach().cpu().float()
                        for k, v in model.state_dict().items()
                    }

                    agg_remote = None
                    total_remote_weight = 0.0
                    successfully_loaded = []

                    # ---- Remote models
                    for f in remote_files:
                        try:
                            ckpt = torch.load(f, map_location="cpu")

                            if "state_dict" not in ckpt:
                                raise KeyError("Missing state_dict")

                            state = ckpt["state_dict"]
                            remote_epoch = float(ckpt.get("epoch", 1.0))

                            if agg_remote is None:
                                agg_remote = {
                                    k: state[k].float() * remote_epoch
                                    for k in state
                                }
                            else:
                                for k in agg_remote:
                                    agg_remote[k] += state[k].float() * remote_epoch

                            total_remote_weight += remote_epoch
                            successfully_loaded.append(f)

                        except Exception as e:
                            logger.error(f"Skipping invalid remote model {f.name}: {e}")

                    if agg_remote is not None and total_remote_weight > 0:
                        # Normalize remote aggregate
                        for k in agg_remote:
                            agg_remote[k] /= total_remote_weight

                        # ---- Final blended update
                        new_state = {}
                        for k in local_state:
                            new_state[k] = (
                                (1.0 - AGG_ALPHA) * local_state[k]
                                + AGG_ALPHA * agg_remote[k]
                            )

                        model.load_state_dict(
                            {k: v.to(device) for k, v in new_state.items()},
                            strict=True,
                        )

                        # Reset optimizer and scheduler
                        optimizer = torch.optim.AdamW(
                            model.parameters(),
                            lr=args.lr,
                            weight_decay=args.weight_decay,
                        )

                        scheduler = torch.optim.lr_scheduler.ReduceLROnPlateau(
                            optimizer, mode="min", factor=0.5, patience=2
                        )

                        logger.info(
                            f"Aggregation complete: "
                            f"local={1.0 - AGG_ALPHA:.2f}, "
                            f"remote={AGG_ALPHA:.2f}, "
                            f"remote_models={len(successfully_loaded)}"
                        )

                        # Move processed files only
                        for f in successfully_loaded:
                            try:
                                f.rename(aggregated_dir / f.name)
                            except Exception as e:
                                logger.error(f"Failed to move {f.name}: {e}")
                else:
                    logger.info("No remote model files found")
            else:
                logger.info("Remote model directory does not exist")


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
