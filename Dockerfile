# -------------------- BUILD GO --------------------
FROM golang:1.25.1 AS builder

WORKDIR /app
COPY . .

WORKDIR /app/server
RUN go mod download
RUN go build -o /flnode


# -------------------- RUNTIME --------------------
FROM python:3.10-slim

WORKDIR /app

# Copy Go binary
COPY --from=builder /flnode /usr/local/bin/flnode

# Copy full project (including Python model code)
COPY . .

# Install Python dependencies
RUN pip install --no-cache-dir grpcio grpcio-tools torch

# Required directories (ensure they exist)
RUN mkdir -p /app/shared/local-models && \
    mkdir -p /app/shared/remote-models

ENTRYPOINT ["/usr/bin/supervisord"]
