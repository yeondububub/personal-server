.PHONY: all web backend build test clean

all: build

web:
	cd web && npm run build

backend:
	go build -o bin/personal-server ./cmd/server

build: web backend

test:
	go test -v ./internal/...
	cd web && npm run typecheck

clean:
	rm -rf bin/ web/dist/
