#!/bin/bash
# deploy.sh — Build and deploy memory-store-mcp
# Called by orchestrator after merge to main.
set -euo pipefail

DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$DIR"

echo "📦 Pulling latest..."
git pull origin main

echo "🔨 Building..."
go build -o memory-store-mcp .

echo "🚀 Restarting service..."
systemctl --user restart memory-store-mcp

echo "✅ Deploy complete: $(date)"
