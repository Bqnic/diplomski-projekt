#!/usr/bin/env bash

set -euo pipefail

if [ $# -lt 1 ]; then
  echo "Usage: $0 <num_nodes>"
  echo "Example: $0 5"
  exit 1
fi

NUM_NODES="$1"

echo "Stopping and removing $NUM_NODES nodes..."

for ((i=0; i<NUM_NODES; i++)); do
  CONTAINER_NAME="node-$i"

  if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    echo "Stopping $CONTAINER_NAME..."
    docker stop "$CONTAINER_NAME" >/dev/null 2>&1 || true

    echo "Removing $CONTAINER_NAME..."
    docker rm "$CONTAINER_NAME" >/dev/null 2>&1 || true
  else
    echo "$CONTAINER_NAME does not exist, skipping..."
  fi
done

echo "All nodes stopped and removed."
