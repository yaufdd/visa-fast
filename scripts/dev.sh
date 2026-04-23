#!/usr/bin/env bash
# Local dev helper — starts Postgres (Docker), applies migrations, sets env
# vars, and launches the Go backend. Frontend runs separately via
# `cd frontend && npm run dev`.
#
# Usage:
#   ./scripts/dev.sh
#
# Prerequisites (one-time, on macOS):
#   brew install tesseract poppler go node
#   mkdir -p docgen/tessdata
#   curl -fL -o docgen/tessdata/rus.traineddata \
#     https://github.com/tesseract-ocr/tessdata_best/raw/main/rus.traineddata
#   curl -fL -o docgen/tessdata/eng.traineddata \
#     https://github.com/tesseract-ocr/tessdata_best/raw/main/eng.traineddata
#   python3 -m pip install --user pytesseract==0.3.13 pdf2image==1.17.0 \
#     pillow==11.1.0 opencv-python-headless==4.10.0.84
#   cp .env.local.example .env.local   # then edit ANTHROPIC_API_KEY
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

if [ ! -f .env.local ]; then
  echo "✘ .env.local not found. Copy .env.local.example first:"
  echo "  cp .env.local.example .env.local"
  echo "  edit ANTHROPIC_API_KEY + any other values"
  exit 1
fi

echo "▶ Starting Postgres (Docker)…"
docker compose up -d

echo "▶ Waiting for Postgres to become healthy…"
for i in {1..20}; do
  if docker compose exec -T db pg_isready -U fuji -d fujitravel >/dev/null 2>&1; then
    echo "  ✓ ready"
    break
  fi
  sleep 1
  if [ "$i" = "20" ]; then
    echo "✘ Postgres failed to become healthy in 20s"
    exit 1
  fi
done

echo "▶ Loading env vars from .env.local…"
# shellcheck disable=SC1091
source .env.local

echo "▶ Running migrations…"
if command -v migrate >/dev/null 2>&1; then
  migrate -path backend/migrations -database "$DATABASE_URL" up
else
  echo "  ⚠  `migrate` CLI not installed — install with:"
  echo "       brew install golang-migrate"
  echo "     or run migrations manually from the backend process on startup"
fi

echo "▶ Starting backend (go run)…"
cd backend
exec go run cmd/server/main.go
