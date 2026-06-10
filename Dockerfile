# Stage 1: Build the Go binary
FROM golang:1.26.4-alpine AS builder
WORKDIR /app
COPY go.mod go.su* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o library-server ./cmd/server/main.go

# Stage 2: Create a lightweight runtime image
FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/library-server .
EXPOSE 8080
CMD ["./library-server"]
