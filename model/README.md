# Lokalno pokretanje skripte (za development)

```bash
python diabetes_train_single.py --epochs 20 --log-weight-stats
```

# Rad modela unutar čvora

1. Lokalno treniranje
2. Svakih N epoha šalje svoj model (serijaliziran u .pt file) serveru preko grpc_client-a.
   **TODO**
3. Svakih N epoha agregira tuđe modele u svoj.
   **TODO**
4. Ispisuje u .json file sve potrebne informacije za vrijeme svog treniranja, uključujući agregiranje tuđih modela.
