COMPOSE := docker compose -f docker-compose.prod.yml

.PHONY: help up down restart build rebuild logs logs-backend logs-web logs-db ps db-only dev-backend dev-frontend deploy clean

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
	GOOGLE_CREDENTIALS_PATH="/Users/yaufdd/Desktop/FUJIT TRAVEL/google-credentials.json" \
	GOOGLE_SHEET_ID="1UH-MK_KsTPbghaAKrx6ous8jFACeL6uEQccsewLcrnM" \
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
