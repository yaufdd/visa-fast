COMPOSE := docker compose -f docker-compose.prod.yml

.PHONY: help up down restart build rebuild logs logs-backend logs-web logs-db ps db-only dev-backend dev-frontend deploy clean audit audit-run audit-full audit-run-full

help:
	@echo "FujiTravel Admin — Make targets"
	@echo ""
	@echo "  make up            — start all services (db + backend + web)"
	@echo "  make down          — stop all services"
	@echo "  make restart       — restart all services"
	@echo "  make build         — build images without starting"
	@echo "  make rebuild       — rebuild images and restart"
	@echo "  make logs          — tail logs for all services"
	@echo "  make logs-backend  — tail backend logs"
	@echo "  make logs-web      — tail web (nginx) logs"
	@echo "  make logs-db       — tail db logs"
	@echo "  make ps            — show container status"
	@echo ""
	@echo "  make db-only       — start only postgres (for local dev)"
	@echo "  make dev-backend   — run Go backend locally (needs db-only)"
	@echo "  make dev-frontend  — run React frontend locally"
	@echo ""
	@echo "  make deploy        — git pull + rebuild + restart (for server)"
	@echo "  make clean         — stop and remove volumes (DANGER: wipes db)"
	@echo ""
	@echo "  make audit          — последний вызов ИИ (кратко: модель, статус, время)"
	@echo "  make audit-run      — все вызовы последней генерации (кратко)"
	@echo "  make audit-full     — последний вызов с полным запросом и ответом"
	@echo "  make audit-run-full — все вызовы последней генерации с полными запросами и ответами"

# ── Docker compose commands ──────────────────────────────────────────────
up:
	$(COMPOSE) up -d

down:
	$(COMPOSE) down

restart:
	$(COMPOSE) restart

build:
	$(COMPOSE) build

rebuild:
	$(COMPOSE) up -d --build

logs:
	$(COMPOSE) logs -f --tail=100

logs-backend:
	$(COMPOSE) logs -f --tail=100 backend

logs-web:
	$(COMPOSE) logs -f --tail=100 web

logs-db:
	$(COMPOSE) logs -f --tail=100 db

ps:
	$(COMPOSE) ps

# ── Local development ────────────────────────────────────────────────────
db-only:
	$(COMPOSE) up -d db

dev-backend:
	cd backend && \
	DATABASE_URL="postgres://fuji:fuji123@localhost:5435/fujitravel?sslmode=disable" \
	ANTHROPIC_API_KEY="$${ANTHROPIC_API_KEY}" \
	PORT=8081 \
	go run cmd/server/main.go

dev-frontend:
	cd frontend && npm run dev

# ── Deployment ───────────────────────────────────────────────────────────
deploy:
	git pull
	$(COMPOSE) up -d --build
	$(COMPOSE) ps

clean:
	$(COMPOSE) down -v

# ── Аудит ИИ-вызовов ─────────────────────────────────────────────────────
# Все команды идут через docker exec к контейнеру `db`, psql на хосте не нужен.

audit:
	@$(COMPOSE) exec -T db psql -U fuji -d fujitravel -c "\
	SELECT started_at, function_name, model, status, duration_ms, \
	       input_tokens, output_tokens, COALESCE(error_msg,'') AS error \
	FROM ai_call_logs \
	ORDER BY started_at DESC \
	LIMIT 1;"

audit-run:
	@$(COMPOSE) exec -T db psql -U fuji -d fujitravel -c "\
	SELECT started_at, function_name, model, status, duration_ms, \
	       input_tokens, output_tokens, COALESCE(error_msg,'') AS error \
	FROM ai_call_logs \
	WHERE generation_id = ( \
	    SELECT generation_id FROM ai_call_logs \
	    ORDER BY started_at DESC LIMIT 1 \
	) \
	ORDER BY started_at ASC;"

audit-full:
	@$(COMPOSE) exec -T db psql -U fuji -d fujitravel -x -c "\
	SELECT started_at, function_name, model, status, duration_ms, \
	       input_tokens, output_tokens, error_msg, \
	       request_json, response_text \
	FROM ai_call_logs \
	ORDER BY started_at DESC \
	LIMIT 1;"

audit-run-full:
	@$(COMPOSE) exec -T db psql -U fuji -d fujitravel -x -c "\
	SELECT started_at, function_name, model, status, duration_ms, \
	       input_tokens, output_tokens, error_msg, \
	       request_json, response_text \
	FROM ai_call_logs \
	WHERE generation_id = ( \
	    SELECT generation_id FROM ai_call_logs \
	    ORDER BY started_at DESC LIMIT 1 \
	) \
	ORDER BY started_at ASC;"
