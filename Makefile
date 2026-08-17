.PHONY: dev dev-down dev-reset dev-seed test lint build docker-build migrate migrate-up migrate-down test-db web-deps web-check checkpoint-phase2 checkpoint-phase3 checkpoint-phase4 checkpoint-phase5 check-phase6a checkpoint-phase6 checkpoint-phase7 checkpoint-phase8 checkpoint-mvp staging-check staging-deploy staging-smoke staging-backup staging-restore-test load-smoke load-baseline prod-build prod-up prod-down prod-restart prod-logs prod-status prod-migrate prod-shell-api prod-shell-db prod-backup prod-restore prod-health prod-deploy

dev:
	docker compose up -d postgres redis
	docker compose run --rm migrate
	docker compose up --build

dev-down:
	docker compose down

dev-reset:
	@echo "WARNING: this removes local Docker volumes including PostgreSQL data."
	docker compose down -v --remove-orphans

migrate: migrate-up

dev-seed:
	docker compose up -d postgres
	docker compose run --rm migrate
	docker compose --profile seed build dev-seed
	docker compose --profile seed run --rm dev-seed

test:
	cd apps/api && go test ./...

lint:
	cd apps/api && test -z "$$(gofmt -l .)" && go vet ./...

build:
	cd apps/api && go build -o /tmp/freelance-api ./cmd/api
	cd worker && go build -o /tmp/freelance-worker ./cmd/worker

docker-build:
	docker compose build api worker web

migrate-up:
	docker compose up -d postgres
	docker compose run --rm migrate

test-db:
	./scripts/test-db.sh

web-deps:
	cd apps/web && node -e 'const fs=require("fs"); if(!fs.existsSync("package-lock.json")) process.exit(1); const p=require("./package.json"), l=require("./package-lock.json"), r=l.packages&&l.packages[""]; if(!r) process.exit(1); for(const k of ["dependencies","devDependencies"]) for(const [n,v] of Object.entries(p[k]||{})) if((r[k]||{})[n]!==v) process.exit(1)' >/dev/null 2>&1 && npm ci --no-audit --no-fund || (echo "package-lock.json is stale; regenerating it from package.json..." && rm -rf node_modules package-lock.json && npm install --no-audit --no-fund)

web-check: web-deps
	cd apps/web && npm audit --audit-level=low
	cd apps/web && npm run lint && npm run typecheck && npm test && npm run build

checkpoint-phase2: lint test build web-check
	cd worker && gofmt -l . && go vet ./... && go test ./...
	./tests/e2e/phase2-checkpoint.sh
	./tests/load/phase2-smoke.sh
	./tests/integration/phase2.sh

checkpoint-phase3: lint test build web-check
	cd worker && gofmt -l . && go vet ./... && go test ./...
	./tests/e2e/phase3-checkpoint.sh
	./tests/load/phase2-smoke.sh
	./tests/integration/phase2.sh

checkpoint-phase4: lint test build web-check
	cd worker && gofmt -l . && go vet ./... && go test ./...
	./tests/e2e/phase4-checkpoint.sh
	./tests/load/phase4-smoke.sh
	./tests/integration/phase4.sh

checkpoint-phase5: lint test build web-check
	cd worker && gofmt -l . && go vet ./... && go test ./...
	./tests/e2e/phase5-checkpoint.sh
	./tests/load/phase5-smoke.sh
	./tests/integration/phase5.sh

check-phase6a: lint build web-check
	cd apps/api && go test -race ./internal/ai ./internal/auth ./internal/projects ./internal/platform/ratelimit ./cmd/api
	./tests/e2e/phase6a-checkpoint.sh
	./tests/load/phase6a-smoke.sh
	./tests/integration/phase6a.sh

checkpoint-phase6: lint test build web-check
	cd apps/api && go test -race ./...
	cd worker && gofmt -l . && go vet ./... && go test -race ./...
	./tests/e2e/phase6-checkpoint.sh
	./tests/load/phase6-smoke.sh
	./tests/integration/phase6.sh

checkpoint-phase7: lint test build web-check
	cd apps/api && go test -race ./...
	cd worker && gofmt -l . && go vet ./... && go test -race ./...
	./tests/e2e/phase7-checkpoint.sh
	./tests/load/phase7-smoke.sh
	./tests/integration/phase7.sh

checkpoint-phase8: lint test build web-check
	cd apps/api && go test -race ./...
	cd worker && gofmt -l . && go vet ./... && go test -race ./...
	./tests/e2e/phase8-checkpoint.sh
	./tests/load/phase8-smoke.sh
	./tests/integration/phase8.sh

checkpoint-mvp: lint test build web-check
	cd apps/api && go test -race ./...
	cd worker && gofmt -l . && go vet ./... && go test -race ./...
	./tests/e2e/phase2-checkpoint.sh && ./tests/e2e/phase3-checkpoint.sh && ./tests/e2e/phase4-checkpoint.sh && ./tests/e2e/phase5-checkpoint.sh
	./tests/e2e/phase6a-checkpoint.sh && ./tests/e2e/phase6-checkpoint.sh && ./tests/e2e/phase7-checkpoint.sh && ./tests/e2e/phase8-checkpoint.sh
	./tests/e2e/mvp-safe-deal-checkpoint.sh
	./tests/load/phase2-smoke.sh && ./tests/load/phase4-smoke.sh && ./tests/load/phase5-smoke.sh && ./tests/load/phase6a-smoke.sh && ./tests/load/phase6-smoke.sh && ./tests/load/phase7-smoke.sh && ./tests/load/phase8-smoke.sh
	./tests/load/mvp-safe-deal-smoke.sh
	./tests/integration/phase2.sh && ./tests/integration/phase4.sh && ./tests/integration/phase5.sh && ./tests/integration/phase6a.sh && ./tests/integration/phase6.sh && ./tests/integration/phase7.sh && ./tests/integration/phase8.sh
	./tests/integration/mvp-safe-deal.sh

load-smoke:
	LOAD_PROFILE=smoke ./tests/load/phase10-k6.sh

load-baseline:
	LOAD_PROFILE=baseline ./tests/load/phase10-k6.sh


	./scripts/smoke-public-details.sh

# ============================
# Production
# ============================

PROD_COMPOSE=docker compose -f docker-compose.production.yml

prod-deploy:
	$(PROD_COMPOSE) build web api worker
	$(PROD_COMPOSE) up -d
	$(PROD_COMPOSE) run --rm migrate
	$(PROD_COMPOSE) ps

prod-build:
	$(PROD_COMPOSE) build web api worker


prod-up:
	$(PROD_COMPOSE) up -d


prod-down:
	$(PROD_COMPOSE) down


prod-restart:
	$(PROD_COMPOSE) restart


prod-logs:
	$(PROD_COMPOSE) logs -f --tail=200


prod-status:
	$(PROD_COMPOSE) ps


prod-migrate:
	$(PROD_COMPOSE) up -d postgres
	$(PROD_COMPOSE) run --rm migrate


prod-health:
	@echo "Checking production containers..."
	$(PROD_COMPOSE) ps
	@echo ""
	@echo "Checking API..."
	curl -fsS http://127.0.0.1:8080/health || true
	@echo ""
	@echo "Checking Web..."
	curl -fsS http://127.0.0.1:3000 || true


prod-shell-api:
	$(PROD_COMPOSE) exec api sh


prod-shell-db:
	$(PROD_COMPOSE) exec postgres psql -U $${POSTGRES_USER} -d $${POSTGRES_DB}


prod-backup:
	@mkdir -p backups
	$(PROD_COMPOSE) exec -T postgres pg_dump \
		-U $${POSTGRES_USER} \
		-d $${POSTGRES_DB} \
		> backups/postgres_$$(date +%Y%m%d_%H%M%S).sql


prod-restore:
	@test -n "$(FILE)" || (echo "Usage: make prod-restore FILE=backup.sql"; exit 1)
	cat $(FILE) | $(PROD_COMPOSE) exec -T postgres psql \
		-U $${POSTGRES_USER} \
		-d $${POSTGRES_DB}
