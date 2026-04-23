# ── build stage ──────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN test -z "$(gofmt -l .)"
RUN go test ./...
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/main.go

# ── final stage ──────────────────────────────────────────────────────────────
FROM alpine:3.19

WORKDIR /app

COPY --from=builder /app/server .
COPY config.yaml .
COPY data/ ./data/

EXPOSE 8080

ENTRYPOINT ["./server"]
