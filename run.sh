#!/usr/bin/env bash
# Loads .env and runs the telegram-forwarder stack via docker compose.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

if [ ! -f .env ]; then
  echo "Missing .env file. Copy your env vars into .env first." >&2
  exit 1
fi

docker compose --env-file .env up --build
