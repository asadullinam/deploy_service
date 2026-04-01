FROM alpine:3.20 AS tools
RUN apk add --no-cache curl && \
    curl -fsSL "https://dl.k8s.io/release/v1.35.0/bin/linux/amd64/kubectl" -o /kubectl && \
    chmod +x /kubectl && \
    curl -fsSL "https://github.com/loft-sh/vcluster/releases/download/v0.31.0/vcluster-linux-amd64" -o /vcluster && \
    chmod +x /vcluster

FROM golang:1.25-alpine AS builder
WORKDIR /app
RUN apk add --no-cache ca-certificates git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /deploy-service ./cmd/server/

FROM alpine:3.20
COPY --from=tools   /kubectl  /usr/local/bin/kubectl
COPY --from=tools   /vcluster /usr/local/bin/vcluster
COPY --from=builder /deploy-service /usr/local/bin/deploy-service
ENTRYPOINT ["/usr/local/bin/deploy-service"]
