# ==========================================
# Stage 1: Builder
# ==========================================
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Download Go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
# CGO_ENABLED=0 creates a static binary (no external C dependencies)
RUN CGO_ENABLED=0 GOOS=linux go build -o kouvenci-backend .

# ==========================================
# Stage 2: Final Production Image
# ==========================================
FROM alpine:latest

WORKDIR /root/

# CRITICAL: Install CA certificates so Go can make HTTPS calls to Google API
RUN apk --no-cache add ca-certificates

# Copy the binary from the builder stage
COPY --from=builder /app/kouvenci-backend .

EXPOSE 8080

CMD ["./kouvenci-backend"]