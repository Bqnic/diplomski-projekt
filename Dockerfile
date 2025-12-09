FROM golang:1.25.1 AS builder

WORKDIR /app

COPY . .

WORKDIR /app/server

RUN go mod download
RUN go build -o /flnode

# ------------------- Runtime image -------------------

FROM debian:bookworm-slim
WORKDIR /app
COPY --from=builder /flnode /usr/local/bin/flnode
COPY . .
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
ENTRYPOINT ["/usr/local/bin/flnode"]
