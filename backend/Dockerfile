FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o /bin/comput3-server ./cmd/server

# ── Runtime ────────────────────────────────────────────────────────────────

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /bin/comput3-server /bin/comput3-server

EXPOSE 8080

USER nobody:nobody

ENTRYPOINT ["/bin/comput3-server"]
