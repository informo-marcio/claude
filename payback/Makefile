# Compila a interface e o servidor em um binário único (bin/payback).
.PHONY: build web test image

build: web
	go build -o bin/payback ./cmd/server

web:
	cd web && npm ci && npm run build

test:
	go vet ./...
	go test ./...
	cd web && npm run typecheck

# Imagem Docker com a revisão do Git como versão.
image:
	docker build --build-arg VERSION=$$(git rev-parse --short HEAD) -t informo/payback:$$(git rev-parse --short HEAD) .
