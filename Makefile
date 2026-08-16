.PHONY: build web backend dev test clean

# Full build: frontend bundle → embedded Go binary.
build: backend

web:
	cd web && npm install && npm run build

backend: web
	go build -o pleumcloud ./cmd/web

# Backend-only build (uses whatever web/dist currently holds).
backend-only:
	go build -o pleumcloud ./cmd/web

# Run Go backend + Vite dev server (hot reload) together.
dev:
	@trap 'kill 0' EXIT; \
		go run ./cmd/web & \
	cd web && npm run dev

test:
	go test ./...

clean:
	rm -f pleumcloud
	rm -rf web/dist
	@mkdir -p web/dist && touch web/dist/.gitkeep
