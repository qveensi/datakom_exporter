# Stage 1: Build the binary
# Updated to 1.23-alpine to match your go.mod requirement
FROM golang:1.23-alpine AS builder

# Set the working directory
WORKDIR /app

# Copy dependency files and download them
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the statically compiled binary
RUN CGO_ENABLED=0 GOOS=linux go build -o datakom-exporter main.go

# Stage 2: Final lightweight image
FROM alpine:latest
RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy only the compiled binary from the builder stage
COPY --from=builder /app/datakom-exporter .

# Default metrics port
EXPOSE 8000

# Run the exporter
CMD ["./datakom-exporter"]