# -------------------- BUILD GO --------------------
FROM golang:1.25.1 AS builder

WORKDIR /app

# Copy go.mod/go.sum first (enables cache)
COPY server/go.mod server/go.sum ./server/
WORKDIR /app/server
RUN go mod download

# Copy the rest of the Go source (invalidates cache ONLY when source changes)
COPY server/ ./
RUN go build -o /flnode


# -------------------- PYTHON RUNTIME --------------------
FROM python:3.10-slim

WORKDIR /app

# Install system deps only once (cached layer)
RUN apt-get update && \
    apt-get install -y supervisor ca-certificates && \
    rm -rf /var/lib/apt/lists/*

# Copy Python requirements first — cached until requirements.txt changes
COPY requirements.txt /tmp/requirements.txt
RUN pip install --no-cache-dir -r /tmp/requirements.txt

# Copy Go binary (small, fast)
COPY --from=builder /flnode /usr/local/bin/flnode

# Copy the rest of the app (does NOT break cached installs)
COPY . .

# Ensure shared folders exist
RUN mkdir -p /app/shared/local-models && \
    mkdir -p /app/shared/remote-models

# Add supervisor config
COPY init/supervisord.conf /etc/supervisor/conf.d/supervisord.conf

ENTRYPOINT ["/usr/bin/supervisord"]
