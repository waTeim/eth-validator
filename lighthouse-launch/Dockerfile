# syntax=docker/dockerfile:1   # enables modern ARG handling

########################
# ▸ Build stage
########################
ARG GO_VERSION=1.23
ARG LH_IMAGE=sigp/lighthouse:v7.1.0
FROM golang:${GO_VERSION} AS builder

WORKDIR /app

# Copy module files first to leverage Docker layer-caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source
COPY . .

# Build a statically-linked binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o lighthouse-launch .

# Choose the Lighthouse image at build-time (defaults to the stable tag)
FROM ${LH_IMAGE}

WORKDIR /app

COPY --from=builder /app/lighthouse-launch /usr/local/bin/lighthouse-launch
COPY docs /app/docs

EXPOSE 5000
ENTRYPOINT ["/usr/local/bin/lighthouse-launch"]
