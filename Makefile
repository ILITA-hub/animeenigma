.PHONY: all build test lint clean generate dev help

# Variables
SERVICES := auth catalog streaming player social rooms scheduler gateway notifications
GO_BUILD_FLAGS := -ldflags="-s -w"
DOCKER_REGISTRY ?= ghcr.io/ilita-hub/animeenigma

# Colors
GREEN  := $(shell tput -Txterm setaf 2)
YELLOW := $(shell tput -Txterm setaf 3)
RESET  := $(shell tput -Txterm sgr0)

help: ## Show this help
	@echo 'Usage:'
	@echo '  ${YELLOW}make${RESET} ${GREEN}<target>${RESET}'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  ${YELLOW}%-20s${RESET} %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# ============================================================================
# Development
# ============================================================================

dev: ## Start local development environment
	docker compose -f docker/docker-compose.yml up -d
	@echo "Development environment started"

dev-down: ## Stop local development environment
	docker compose -f docker/docker-compose.yml down

dev-logs: ## Show logs from development environment
	docker compose -f docker/docker-compose.yml logs -f

# ============================================================================
# Build
# ============================================================================

all: generate lint test build ## Run all checks and build

build: $(addprefix build-,$(SERVICES)) ## Build all services
	@echo "All services built successfully"

build-%: ## Build a specific service
	@echo "Building $*..."
	cd services/$* && go build $(GO_BUILD_FLAGS) -o ../../bin/$*-api ./cmd/$*-api

build-tools: ## Build all tools
	cd tools/migrator && go build $(GO_BUILD_FLAGS) -o ../../bin/migrator .
	cd tools/sync-cli && go build $(GO_BUILD_FLAGS) -o ../../bin/sync-cli .

# ============================================================================
# Testing
# ============================================================================

test: ## Run all tests
	@echo "Running tests..."
	@for svc in $(SERVICES); do \
		echo "Testing $$svc..."; \
		cd services/$$svc && go test ./... -cover && cd ../..; \
	done

test-%: ## Run tests for a specific service
	cd services/$* && go test ./... -cover -v

test-integration: ## Run integration tests
	@echo "Running integration tests..."
	@for svc in $(SERVICES); do \
		echo "Integration testing $$svc..."; \
		cd services/$$svc && go test ./tests/integration/... -tags=integration && cd ../..; \
	done

test-e2e: ## Run end-to-end tests
	cd frontend/web && pnpm test:e2e

# ============================================================================
# Code Quality
# ============================================================================

lint: lint-go lint-proto lint-frontend ## Run all linters

lint-go: ## Run Go linter (matches CI)
	@echo "Linting Go code..."
	@for mod in libs/* services/auth services/catalog services/gateway services/player services/rooms services/scheduler services/streaming; do \
		if [ -f "$$mod/go.mod" ]; then \
			echo "Linting $$mod..."; \
			(cd "$$mod" && golangci-lint run ./...) || exit 1; \
		fi; \
	done
	@echo "All Go modules passed linting"

lint-proto: ## Lint protobuf files
	buf lint api/proto

lint-frontend: ## Lint frontend code
	cd frontend/web && bun lint

fmt: ## Format all code
	@echo "Formatting Go code..."
	gofmt -s -w .
	@echo "Formatting frontend code..."
	cd frontend/web && pnpm format

# ============================================================================
# Code Generation
# ============================================================================

generate: generate-proto generate-openapi generate-graphql ## Generate all code

generate-proto: ## Generate protobuf code
	@echo "Generating protobuf code..."
	buf generate api/proto

generate-openapi: ## Generate OpenAPI clients
	@echo "Generating OpenAPI code..."
	./scripts/generate-api.sh

generate-graphql: ## Generate GraphQL code
	@echo "Generating GraphQL code..."
	cd frontend/web && pnpm graphql-codegen

# ============================================================================
# Database
# ============================================================================

migrate-up: ## Run database migrations
	./bin/migrator up

migrate-down: ## Rollback last migration
	./bin/migrator down

migrate-create: ## Create new migration (usage: make migrate-create NAME=create_users)
	./bin/migrator create $(NAME)

seed: ## Seed database with sample data
	go run tools/db-seeder/main.go

# ============================================================================
# Docker
# ============================================================================

docker-build: $(addprefix docker-build-,$(SERVICES)) ## Build all Docker images
	@echo "All Docker images built successfully"

docker-build-%: ## Build Docker image for a specific service
	@echo "Building Docker image for $*..."
	docker build -t $(DOCKER_REGISTRY)/$*:latest -f services/$*/Dockerfile .

docker-push: $(addprefix docker-push-,$(SERVICES)) ## Push all Docker images
	@echo "All Docker images pushed successfully"

docker-push-%: ## Push Docker image for a specific service
	docker push $(DOCKER_REGISTRY)/$*:latest

# ============================================================================
# Kubernetes / Helm
# ============================================================================

helm-deps: ## Update Helm dependencies
	@for chart in infra/helm/*/; do \
		echo "Updating dependencies for $$chart..."; \
		helm dependency update $$chart; \
	done

helm-lint: ## Lint Helm charts
	@for chart in infra/helm/*/; do \
		echo "Linting $$chart..."; \
		helm lint $$chart; \
	done

deploy-dev: ## Deploy to development environment
	kubectl config use-context dev
	./scripts/deploy.sh dev

deploy-prod: ## Deploy to production environment
	@echo "Are you sure you want to deploy to production? [y/N] " && read ans && [ $${ans:-N} = y ]
	kubectl config use-context prod
	./scripts/deploy.sh prod

# ============================================================================
# Sync / Import
# ============================================================================

sync-shikimori: ## Sync anime data from Shikimori
	./bin/sync-cli shikimori --full

sync-mal: ## Sync anime data from MyAnimeList
	./bin/sync-cli mal --full

sync-incremental: ## Run incremental sync from all sources
	./bin/sync-cli all --incremental

# ============================================================================
# Cleanup
# ============================================================================

clean: ## Clean build artifacts
	rm -rf bin/
	rm -rf frontend/web/dist/
	@for svc in $(SERVICES); do \
		rm -rf services/$$svc/tmp/; \
	done

clean-docker: ## Clean Docker resources
	docker compose -f docker/docker-compose.yml down -v --rmi local

# ============================================================================
# Frontend
# ============================================================================

frontend-install: ## Install frontend dependencies
	cd frontend/web && bun install

frontend-dev: ## Start frontend development server
	cd frontend/web && bun dev

frontend-build: ## Build frontend for production
	cd frontend/web && bun run build

frontend-preview: ## Preview production build
	cd frontend/web && bun run preview
