.PHONY: all build test lint clean generate dev help \
	k8s-apply k8s-delete k8s-diff k8s-wait k8s-status k8s-restart k8s-logs k8s-port-forward \
	deploy-docker deploy-docker-pull deploy-k8s deploy-dev deploy-staging deploy-prod \
	migrate migrate-down migrate-force migrate-version migrate-auth migrate-catalog migrate-player migrate-rooms migrate-all migrate-create migrate-status db-shell \
	redeploy-all redeploy-web

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
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z0-9_-]+:.*?## / {printf "  ${YELLOW}%-20s${RESET} %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# ============================================================================
# Development
# ============================================================================

dev: ## Start local development environment
	docker-compose -f docker/docker-compose.yml up -d
	@echo "Development environment started"

dev-down: ## Stop local development environment
	docker-compose -f docker/docker-compose.yml down

dev-logs: ## Show logs from development environment
	docker-compose -f docker/docker-compose.yml logs -f

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
# Database (golang-migrate)
# ============================================================================

# Database connection settings (can be overridden via environment)
DB_HOST ?= 172.29.0.2
DB_PORT ?= 5432
DB_USER ?= postgres
DB_PASSWORD ?= postgres

# Database URL helper
define DB_URL
postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/animeenigma_$(1)?sslmode=disable
endef

# Run migrations for a specific service
migrate: ## Run migrations for SERVICE (usage: make migrate SERVICE=auth)
	@if [ -z "$(SERVICE)" ]; then echo "Usage: make migrate SERVICE=auth|catalog|player|rooms"; exit 1; fi
	migrate -path services/$(SERVICE)/migrations -database "$(call DB_URL,$(SERVICE))" up

migrate-down: ## Rollback one migration for SERVICE (usage: make migrate-down SERVICE=auth)
	@if [ -z "$(SERVICE)" ]; then echo "Usage: make migrate-down SERVICE=auth|catalog|player|rooms"; exit 1; fi
	migrate -path services/$(SERVICE)/migrations -database "$(call DB_URL,$(SERVICE))" down 1

migrate-force: ## Force set migration version (usage: make migrate-force SERVICE=auth VERSION=3)
	@if [ -z "$(SERVICE)" ] || [ -z "$(VERSION)" ]; then echo "Usage: make migrate-force SERVICE=auth VERSION=3"; exit 1; fi
	migrate -path services/$(SERVICE)/migrations -database "$(call DB_URL,$(SERVICE))" force $(VERSION)

migrate-version: ## Show current migration version for SERVICE
	@if [ -z "$(SERVICE)" ]; then echo "Usage: make migrate-version SERVICE=auth|catalog|player|rooms"; exit 1; fi
	migrate -path services/$(SERVICE)/migrations -database "$(call DB_URL,$(SERVICE))" version

migrate-auth: ## Run auth service migrations
	@$(MAKE) migrate SERVICE=auth

migrate-catalog: ## Run catalog service migrations
	@$(MAKE) migrate SERVICE=catalog

migrate-player: ## Run player service migrations
	@$(MAKE) migrate SERVICE=player

migrate-rooms: ## Run rooms service migrations
	@$(MAKE) migrate SERVICE=rooms

migrate-all: migrate-auth migrate-catalog migrate-player migrate-rooms ## Run all migrations
	@echo "All migrations completed"

migrate-create: ## Create new migration (usage: make migrate-create SERVICE=auth NAME=add_field)
	@if [ -z "$(SERVICE)" ] || [ -z "$(NAME)" ]; then echo "Usage: make migrate-create SERVICE=auth NAME=add_field"; exit 1; fi
	migrate create -ext sql -dir services/$(SERVICE)/migrations -seq $(NAME)

migrate-status: ## Show migration status for all services
	@echo "=== Auth ===" && migrate -path services/auth/migrations -database "$(call DB_URL,auth)" version 2>&1 || true
	@echo "=== Catalog ===" && migrate -path services/catalog/migrations -database "$(call DB_URL,catalog)" version 2>&1 || true
	@echo "=== Player ===" && migrate -path services/player/migrations -database "$(call DB_URL,player)" version 2>&1 || true
	@echo "=== Rooms ===" && migrate -path services/rooms/migrations -database "$(call DB_URL,rooms)" version 2>&1 || true

db-shell: ## Open psql shell to a database (usage: make db-shell SERVICE=auth)
	@if [ -z "$(SERVICE)" ]; then echo "Usage: make db-shell SERVICE=auth|catalog|player|rooms"; exit 1; fi
	PGPASSWORD=$(DB_PASSWORD) psql -h $(DB_HOST) -p $(DB_PORT) -U $(DB_USER) -d animeenigma_$(SERVICE)

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
# Redeploy (local docker-compose)
# ============================================================================

redeploy-web: ## Rebuild and restart web frontend
	@echo "Rebuilding web frontend..."
	docker-compose -f docker/docker-compose.yml build web
	docker stop animeenigma-web || true
	docker rm animeenigma-web || true
	docker-compose -f docker/docker-compose.yml up -d --no-deps web
	@echo "Web frontend redeployed"

redeploy-%: ## Rebuild and restart a service (e.g., make redeploy-gateway)
	./deploy/scripts/redeploy.sh $*
	@if [ "$*" = "catalog" ]; then \
		echo "Purging HiAnime caches..."; \
		docker exec $$(docker ps -qf "name=redis" | head -1) redis-cli KEYS "hianime:*" | \
			xargs -r docker exec -i $$(docker ps -qf "name=redis" | head -1) redis-cli DEL 2>/dev/null || true; \
	fi

redeploy-all: ## Rebuild and restart all backend services
	./deploy/scripts/redeploy.sh

restart-%: ## Restart a service without rebuilding (e.g., make restart-grafana)
	cd docker && docker compose restart $*

logs-%: ## Follow logs for a service (e.g., make logs-gateway)
	cd docker && docker compose logs -f $*

# ============================================================================
# Kubernetes / Kustomize
# ============================================================================

KUSTOMIZE_BASE := deploy/kustomize/base
NAMESPACE := animeenigma

k8s-apply: ## Apply Kustomize manifests to current cluster
	kubectl apply -k $(KUSTOMIZE_BASE)
	@echo "Waiting for deployments to be ready..."
	@$(MAKE) k8s-wait

k8s-delete: ## Delete all resources from cluster
	kubectl delete -k $(KUSTOMIZE_BASE) --ignore-not-found

k8s-diff: ## Show diff between local and cluster state
	kubectl diff -k $(KUSTOMIZE_BASE) || true

k8s-wait: ## Wait for all deployments to be ready
	@for deploy in gateway auth catalog streaming player rooms scheduler web; do \
		echo "Waiting for $$deploy..."; \
		kubectl rollout status deployment/$$deploy -n $(NAMESPACE) --timeout=120s || true; \
	done

k8s-status: ## Show status of all deployments
	@echo "=== Deployments ==="
	kubectl get deployments -n $(NAMESPACE)
	@echo ""
	@echo "=== Pods ==="
	kubectl get pods -n $(NAMESPACE)
	@echo ""
	@echo "=== Services ==="
	kubectl get services -n $(NAMESPACE)

k8s-restart: ## Restart all deployments
	kubectl rollout restart deployment -n $(NAMESPACE)

k8s-restart-%: ## Restart a specific deployment (e.g., make k8s-restart-catalog)
	kubectl rollout restart deployment/$* -n $(NAMESPACE)

k8s-logs: ## Show logs from all pods (last 100 lines)
	kubectl logs -n $(NAMESPACE) -l app.kubernetes.io/part-of=animeenigma --tail=100 --all-containers

k8s-logs-%: ## Show logs from a specific service (e.g., make k8s-logs-catalog)
	kubectl logs -n $(NAMESPACE) -l app=$* --tail=200 -f

k8s-shell-%: ## Open shell in a service pod (e.g., make k8s-shell-catalog)
	kubectl exec -it -n $(NAMESPACE) deployment/$* -- /bin/sh

k8s-port-forward: ## Port-forward gateway to localhost:8000
	kubectl port-forward -n $(NAMESPACE) svc/gateway 8000:8000

k8s-port-forward-grafana: ## Port-forward Grafana to localhost:3000
	kubectl port-forward -n monitoring svc/grafana 3000:3000

k8s-port-forward-prometheus: ## Port-forward Prometheus to localhost:9090
	kubectl port-forward -n monitoring svc/prometheus 9090:9090

k8s-monitoring-status: ## Show status of monitoring stack
	@echo "=== Monitoring Namespace ==="
	kubectl get all -n monitoring

# ============================================================================
# Deploy Commands
# ============================================================================

deploy-docker: ## Deploy using docker-compose (production build)
	docker-compose -f docker/docker-compose.yml up -d --build
	@echo "Deployed with docker-compose"

deploy-docker-pull: ## Pull latest images and deploy
	docker-compose -f docker/docker-compose.yml pull
	docker-compose -f docker/docker-compose.yml up -d
	@echo "Deployed with latest images"

deploy-k8s: docker-build docker-push k8s-apply ## Build, push images and deploy to Kubernetes
	@echo "Full Kubernetes deployment complete"

deploy-dev: ## Deploy to development (docker-compose)
	@$(MAKE) deploy-docker

deploy-staging: ## Deploy to staging Kubernetes cluster
	kubectl config use-context staging
	@$(MAKE) k8s-apply
	@echo "Deployed to staging"

deploy-prod: ## Deploy to production Kubernetes cluster (with confirmation)
	@echo "WARNING: You are about to deploy to PRODUCTION"
	@echo "Are you sure? [y/N] " && read ans && [ $${ans:-N} = y ]
	kubectl config use-context prod
	@$(MAKE) k8s-apply
	@echo "Deployed to production"

rollback-%: ## Rollback a deployment (e.g., make rollback-catalog)
	kubectl rollout undo deployment/$* -n $(NAMESPACE)
	kubectl rollout status deployment/$* -n $(NAMESPACE)

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
	docker-compose -f docker/docker-compose.yml down -v --rmi local

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

# ============================================================================
# Health & Info
# ============================================================================

health: ## Check health of all services (docker-compose)
	@echo "Checking service health..."
	@curl -sf http://localhost:8000/health > /dev/null && echo "✓ gateway:8000" || echo "✗ gateway:8000"
	@curl -sf http://localhost:8080/health > /dev/null && echo "✓ auth:8080" || echo "✗ auth:8080"
	@curl -sf http://localhost:8081/health > /dev/null && echo "✓ catalog:8081" || echo "✗ catalog:8081"
	@curl -sf http://localhost:8082/health > /dev/null && echo "✓ streaming:8082" || echo "✗ streaming:8082"
	@curl -sf http://localhost:8083/health > /dev/null && echo "✓ player:8083" || echo "✗ player:8083"
	@curl -sf http://localhost:8084/health > /dev/null && echo "✓ rooms:8084" || echo "✗ rooms:8084"
	@curl -sf http://localhost:8085/health > /dev/null && echo "✓ scheduler:8085" || echo "✗ scheduler:8085"

metrics: ## Fetch metrics from all services
	@echo "=== Gateway Metrics ==="
	@curl -sf http://localhost:8000/metrics | head -20 || echo "Gateway not available"
	@echo ""

info: ## Show version info
	@echo "Docker Compose Services:"
	@docker-compose -f docker/docker-compose.yml ps 2>/dev/null || echo "Docker Compose not running"
	@echo ""
	@echo "Docker Registry: $(DOCKER_REGISTRY)"
	@echo "Services: $(SERVICES)"

ps: ## Show running containers
	docker-compose -f docker/docker-compose.yml ps
