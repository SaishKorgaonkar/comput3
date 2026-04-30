#!/usr/bin/env bash
# deploy-contracts.sh — Compile and deploy ProviderRegistry, DeploymentEscrow, JobAuction
# Usage: ./scripts/deploy-contracts.sh
#
# Reads PRIVATE_KEY and RPC_URL from the environment (or .env).
# Writes deployed addresses to contracts/deployments/baseSepolia.json

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CONTRACTS_DIR="$ROOT_DIR/contracts"

# Source .env if it exists
if [[ -f "$ROOT_DIR/.env" ]]; then
  set -o allexport
  # shellcheck disable=SC1091
  source "$ROOT_DIR/.env"
  set +o allexport
fi

: "${PRIVATE_KEY:?PRIVATE_KEY is required}"
: "${RPC_URL:=https://sepolia.base.org}"

cd "$CONTRACTS_DIR"

echo "==> Installing contract dependencies…"
npm install --silent

echo "==> Compiling contracts…"
npx hardhat compile --quiet

echo "==> Deploying to Base Sepolia ($RPC_URL)…"
PRIVATE_KEY="$PRIVATE_KEY" \
RPC_URL="$RPC_URL" \
  npx hardhat run scripts/deploy.ts --network baseSepolia

echo "==> Exporting ABIs…"
npx hardhat run scripts/export-abis.ts --network baseSepolia

echo ""
echo "✓ Deployment complete. Update .env with the addresses printed above."
