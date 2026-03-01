.PHONY: dev dev-backend dev-frontend build build-backend build-frontend test lint clean

# Start both backend (Air live-reload) and frontend (Nuxt HMR) in parallel
dev:
	@echo "Starting backend (Air) and frontend (Nuxt) dev servers..."
	$(MAKE) dev-backend &
	$(MAKE) dev-frontend &
	wait

# Start backend with Air live-reload (port 2187)
dev-backend:
	cd backend && air

# Start frontend Nuxt dev server (port 3000)
dev-frontend:
	cd frontend && pnpm run dev

# Build both backend and frontend for production
build: build-backend build-frontend

# Build backend Go binary
build-backend:
	cd backend && go build -o capacitarr .

# Build frontend Nuxt output
build-frontend:
	cd frontend && pnpm run build

# Run Go tests
test:
	cd backend && go test ./...

# Run golangci-lint
lint:
	cd backend && golangci-lint run

# Remove build artifacts
clean:
	rm -f backend/capacitarr
	rm -rf backend/tmp
	rm -rf frontend/.output
	rm -rf frontend/.nuxt
