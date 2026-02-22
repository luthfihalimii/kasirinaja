.PHONY: help dev build test lint format clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# Backend targets
backend-test: ## Run backend unit tests
	cd backend && go test ./...

backend-lint: ## Run golangci-lint on backend
	cd backend && golangci-lint run

backend-build: ## Build backend binary
	cd backend && go build -o bin/server ./cmd/server

backend-vuln: ## Run govulncheck on backend
	cd backend && govulncheck ./...

# Frontend targets
frontend-install: ## Install frontend dependencies
	cd frontend && bun install

frontend-test: ## Run frontend unit tests
	cd frontend && bun run test

frontend-lint: ## Run ESLint on frontend
	cd frontend && bun run lint

frontend-format: ## Format frontend code with Prettier
	cd frontend && bun run format

frontend-format-check: ## Check frontend formatting
	cd frontend && bun run format:check

frontend-build: ## Build frontend
	cd frontend && bun run build

# Combined targets
test: backend-test frontend-test ## Run all tests

lint: backend-lint frontend-lint ## Run all linters

# Docker targets
up: ## Start all services with docker-compose
	docker compose up --build

down: ## Stop all services
	docker compose down

dev: ## Start services in dev mode (detached postgres/redis, local backend/frontend)
	docker compose up -d postgres redis
	@echo "Postgres and Redis running. Start backend: cd backend && go run ./cmd/server"
	@echo "Start frontend: cd frontend && bun run dev"
