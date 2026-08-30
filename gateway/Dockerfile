# Stage 1: Build the Go binary
FROM golang:1.24-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git gcc musl-dev

COPY go.mod go.sum ./
RUN go mod download

COPY . ./

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o sentinel-gateway .

# Stage 2: Minimal runtime image
FROM alpine:3.20

WORKDIR /app

RUN apk --no-cache add ca-certificates tzdata && \
    addgroup -S sentinel && adduser -S sentinel -G sentinel && \
    mkdir -p /app/inbox /app/data && \
    chown -R sentinel:sentinel /app

COPY --from=builder /app/sentinel-gateway /app/sentinel-gateway
COPY migrations/ /app/migrations/
COPY migrations_postgres/ /app/migrations_postgres/

USER sentinel

EXPOSE 8080

VOLUME ["/app/inbox", "/app/data"]

ENTRYPOINT ["/app/sentinel-gateway"]
