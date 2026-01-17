#!/usr/bin/env bash

set -euo pipefail

if [ $# -lt 1 ]; then
  echo "Korištenje: $0 <num_nodes>"
  echo "Primjer: $0 5"
  exit 1
fi

NUM_NODES="$1"

IMAGE="server-node"
BASE_PORT=9001
SENDING_EPOCH=3

echo "Starting $NUM_NODES nodes"

BOOTSTRAP_CONTAINER=""

# 1. Start bootstrap node
echo "Starting bootstrap node on port $BASE_PORT"

BOOTSTRAP_CONTAINER=$(docker run -d \
  --name "node-0" \
  -p "$BASE_PORT:$BASE_PORT" \
  -e NUM_NODES="$NUM_NODES" \
  -e SENDING_EPOCH="$SENDING_EPOCH" \
  -e NODE_NAME="node-0" \
  -e FL_LISTEN="/ip4/0.0.0.0/tcp/$BASE_PORT" \
  "$IMAGE")

# Wait briefly for libp2p host to come up
sleep 2

# Resolve bootstrap container IP
BOOTSTRAP_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$BOOTSTRAP_CONTAINER")

if [ -z "$BOOTSTRAP_IP" ]; then
  echo "Failed to resolve bootstrap container IP"
  exit 1
fi

echo "Bootstrap container IP: $BOOTSTRAP_IP"
echo "Bootstrap container ID: $BOOTSTRAP_CONTAINER"

# Extract Peer ID from bootstrap logs
echo "Waiting for bootstrap peer ID..."
sleep 2

BOOTSTRAP_PEER_ID=$(docker logs "$BOOTSTRAP_CONTAINER" 2>&1 | \
  grep -Eo '12D3Koo[a-zA-Z0-9]+' | head -n 1)

if [ -z "$BOOTSTRAP_PEER_ID" ]; then
  echo "Failed to extract bootstrap peer ID from logs"
  exit 1
fi

BOOTSTRAP_MULTIADDR="/ip4/$BOOTSTRAP_IP/tcp/$BASE_PORT/p2p/$BOOTSTRAP_PEER_ID"

echo "Bootstrap multiaddr:"
echo "  $BOOTSTRAP_MULTIADDR"

# 2. Start remaining nodes
for ((i=1; i<=$NUM_NODES-1; i++)); do
  PORT=$((BASE_PORT + i))

  echo "Starting node $i on port $PORT"

  docker run -d \
    --name "node-$i" \
    -p "$PORT:$PORT" \
    -e NODE_NAME="node-$i" \
    -e NUM_NODES="$NUM_NODES" \
    -e SENDING_EPOCH="$SENDING_EPOCH" \
    -e FL_LISTEN="/ip4/0.0.0.0/tcp/$PORT" \
    -e BOOTSTRAP_PEER="$BOOTSTRAP_MULTIADDR" \
    "$IMAGE"
done

echo "All nodes started successfully."
