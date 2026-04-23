# Makefile for Apica Backend

.PHONY: build run clean docker.build docker.run docker.stop

PORT := $(shell grep '^port:' config.yaml | sed 's/port: "(.*)"/\1/')
BUILDER_IMAGE := apica-builder
# Go commands
build:
	go build -o main ./cmd/main.go

run:
	go run ./cmd/main.go

clean:
	rm -f main

# Docker commands
docker.build:
	docker build -f build/Dockerfile.builder -t apica-builder .
	docker build --build-arg BUILDER_IMAGE=$(BUILDER_IMAGE) --build-arg PORT=$(PORT) -f build/Dockerfile -t apica-backend .

docker.run:
	docker run -p $(PORT):$(PORT) --name apica-backend-container apica-backend

docker.stop:
	docker stop apica-backend-container || true
	docker rm apica-backend-container || true

# Combined commands
docker.run.bg:
	docker run -d -p $(PORT):$(PORT) --name apica-backend-container apica-backend
