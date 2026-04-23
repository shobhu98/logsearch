# Makefile for  Backend

.PHONY: build run clean fmt test docker.build docker.run docker.stop

PORT := $(shell grep '^port:' config.yaml | sed 's/port: "(.*)"/\1/')
BUILDER_IMAGE := log-builder
# Go commands
build:
	go build -o main ./cmd/main.go

run:
	go run ./cmd/main.go

fmt:
	go fmt ./...

test:
	go test ./...

clean:
	rm -f main

# Docker commands
docker.build:
	docker build -f build/Dockerfile.builder -t log-builder .
	docker build --build-arg BUILDER_IMAGE=$(BUILDER_IMAGE) --build-arg PORT=$(PORT) -f build/Dockerfile -t log-backend .

docker.run:
	docker run -p $(PORT):$(PORT) --name log-backend-container log-backend

docker.stop:
	docker stop log-backend-container || true
	docker rm log-backend-container || true

# Combined commands
docker.run.bg:
	docker run -d -p $(PORT):$(PORT) --name log-backend-container log-backend
