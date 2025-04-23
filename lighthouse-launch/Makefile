IMAGE ?= wateim/lighthouse-launch
TAG ?= v1.0.0

.PHONY: all docs build push clean

# Generate swagger docs, then build and push.
all: build push

# Generate OpenAPI documentation via swag.
docs:
	swag init --parseDependency --parseInternal --parseDepth=1

# Build the Docker image (depends on generating swagger docs).
build: docs
	docker build --platform=linux/amd64 -t $(IMAGE):$(TAG) .

# Push the Docker image.
push:
	docker push $(IMAGE):$(TAG)

# Clean up the built image.
clean:
	docker rmi $(IMAGE):$(TAG)
