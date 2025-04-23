# Build stage: compile the Go program using Go 1.23.
FROM golang:1.23 AS builder
WORKDIR /app
# Copy module files and download dependencies.
COPY go.mod go.sum ./
RUN go mod download
# Copy the rest of the source code.
COPY . .
# Build the static binary named "lighthouse-launch".
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o lighthouse-launch .

# Final stage: use the official Lighthouse base image.
FROM sigp/lighthouse:v7.0.0
WORKDIR /app
# Copy the compiled binary and pre-generated swagger docs.
COPY --from=builder /app/lighthouse-launch /usr/local/bin/lighthouse-launch
COPY docs /app/docs
EXPOSE 5000
ENTRYPOINT ["/usr/local/bin/lighthouse-launch"]
