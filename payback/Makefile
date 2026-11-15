# Compila a interface e o servidor em um binário único (bin/payback).
.PHONY: build web test

build: web
	go build -o bin/payback ./cmd/server

web:
	cd web && npm ci && npm run build

test:
	go vet ./...
	go test ./...
	cd web && npm run typecheck
