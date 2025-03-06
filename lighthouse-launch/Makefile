# Set your image name and tag here.
IMAGE ?= wateim/lighthouse-launch
TAG ?= latest

.PHONY: all build push clean

# Default target builds and then pushes the image.
all: build push

# Build the Docker image using the Dockerfile in the current directory.
build:
	docker build --platform=linux/amd64 -t $(IMAGE):$(TAG) .

# Push the Docker image to your registry.
push:
	docker push $(IMAGE):$(TAG)

# Clean up by removing the built image.
clean:
	docker rmi $(IMAGE):$(TAG)
