# Build payment service binary
FROM golang:1.26.4-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o payment-server ./payment/cmd/main.go

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/payment-server .
EXPOSE 50051
CMD ["./payment-server"]
