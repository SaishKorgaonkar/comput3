#!/usr/bin/env bash
# register-eas-schema.sh — Register the audit log EAS schema on Base Sepolia
# Usage: ./scripts/register-eas-schema.sh
#
# Reads PRIVATE_KEY, RPC_URL, EAS_CONTRACT_ADDRESS from the environment (or .env).
# Prints the schema UID — copy it to EAS_SCHEMA_UID in .env.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CONTRACTS_DIR="$ROOT_DIR/contracts"

if [[ -f "$ROOT_DIR/.env" ]]; then
  set -o allexport
  # shellcheck disable=SC1091
  source "$ROOT_DIR/.env"
  set +o allexport
fi

: "${DEPLOYER_PRIVATE_KEY:?DEPLOYER_PRIVATE_KEY is required}"
: "${ETH_SEPOLIA_RPC_URL:=https://rpc.sepolia.org}"
: "${EAS_CONTRACT_ADDRESS:=0xC2679fBD37d54388Ce493F1DB75320D236e1815e}"

cd "$CONTRACTS_DIR"

echo "==> Registering EAS schema…"
DEPLOYER_PRIVATE_KEY="$DEPLOYER_PRIVATE_KEY" \
ETH_SEPOLIA_RPC_URL="$ETH_SEPOLIA_RPC_URL" \
EAS_CONTRACT_ADDRESS="$EAS_CONTRACT_ADDRESS" \
  npx hardhat run scripts/register-eas-schema.ts --network ethSepolia

echo "✓ Copy the schema UID above into EAS_SCHEMA_UID in your .env"
