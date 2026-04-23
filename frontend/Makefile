IMAGE  := log-search-ui
PORT   := 3000

# Use cmd.exe on Windows so variable expansion works correctly
ifeq ($(OS),Windows_NT)
  SHELL := cmd.exe
  .SHELLFLAGS := /c
endif

.PHONY: install dev build preview lint \
        docker-build docker-run docker-stop docker-clean help

## ── local dev ──────────────────────────────────────────────────────────
install:
	npm ci

dev:
	npm run dev

build:
	npm run build

preview:
	npm run preview

lint:
	npm run lint

## ── docker ─────────────────────────────────────────────────────────────
docker-build:
	docker build -f build/Dockerfile -t $(IMAGE) .

docker-run:
	docker run -d --name $(IMAGE) -p $(PORT):80 $(IMAGE)
	@echo Running at http://localhost:$(PORT)

docker-stop:
	docker stop $(IMAGE)
	docker rm $(IMAGE)

docker-clean: docker-stop
	docker rmi $(IMAGE)

## ── help ───────────────────────────────────────────────────────────────
help:
	@echo.
	@echo   install       install npm dependencies
	@echo   dev           start vite dev server
	@echo   build         production build  (output: dist/)
	@echo   preview       preview production build locally
	@echo   lint          run eslint
	@echo.
	@echo   docker-build  build docker image  ($(IMAGE))
	@echo   docker-run    run container on port $(PORT)
	@echo   docker-stop   stop and remove container
	@echo   docker-clean  stop container + remove image
	@echo.
