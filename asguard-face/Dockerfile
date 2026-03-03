# ---- Builder stage ----
FROM golang:1.25-bookworm AS builder

# Install build dependencies
RUN apt-get update && apt-get install -y \
    libdlib-dev \
    libatlas-base-dev \
    libjpeg62-turbo-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY main.go .
RUN CGO_ENABLED=1 GOOS=linux go build -o face-service main.go

# ---- Runtime stage ----
FROM debian:bookworm-slim

# Install runtime libraries (smaller than dev packages)
RUN apt-get update && apt-get install -y \
    libdlib19.1 \
    libopenblas0 \
    liblapack3 \
    libjpeg62-turbo \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
# Copy the compiled binary from the builder stage
COPY --from=builder /app/face-service /app/face-service

# The command to run when the container starts
CMD ["/app/face-service"]