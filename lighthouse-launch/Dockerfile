# Build stage: compile the Go program
FROM golang:1.23 AS builder
WORKDIR /app
# Copy your Go source code (e.g. main.go) into the container.
COPY . .
# Build a static binary named "entrypoint".
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o lighthouse-launch .

# Final stage: use the official Lighthouse image as the base.
FROM sigp/lighthouse:latest
WORKDIR /app
# Copy the compiled binary from the builder stage.
COPY --from=builder /app/lighthouse-launch /usr/local/bin/lighthouse-launch
# (Optional) Expose the port your HTTP server will listen on.
EXPOSE 5000
# Set the container's entrypoint to your binary.
ENTRYPOINT ["/usr/local/bin/lighthouse-launch"]
