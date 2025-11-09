FROM golang:1.25.1 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /flnode

FROM debian:bookworm-slim
WORKDIR /app
COPY --from=builder /flnode /usr/local/bin/flnode
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
ENTRYPOINT ["/usr/local/bin/flnode"]
