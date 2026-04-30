# COMPUT3

**Trustless decentralized compute for AI agents — private by default, verifiable by design.**

Built for ETHGlobal OpenAgents.

---

## Problem

AI agents need to execute code, run workloads, and process sensitive data — but today they rely entirely on centralized infrastructure. There is no way to verify what actually ran, providers can read user data, and there is no economic accountability for compute providers.

## Solution

COMPUT3 is a decentralized compute network where:

- Users submit tasks to an AI agent (Claude claude-opus-4-5)
- The agent selects a provider node from an on-chain registry (Ethereum Sepolia)
- Execution happens inside an **encrypted container** (LUKS2 AES-256) — the provider cannot read user data
- Every action is **logged, SHA256-hashed, and Merkle-verified**
- A Merkle root of execution is submitted as an **EAS attestation** on Ethereum Sepolia
- Providers stake ETH as collateral and can be slashed for misbehavior
- Payments stream as USDC micro-payments via the **x402 protocol** (EIP-3009)

---

## Architecture

```
User
 │
 ▼
AI Agent (Claude claude-opus-4-5)
 │  - analyze repo / task
 │  - select provider (on-chain registry)
 │  - generate execution plan → user confirms
 ▼
Provider Node  (COMPUT3 Go backend)
 │  - spins Docker container
 │  - mounts LUKS2 AES-256 encrypted volume
 │  - executes steps, logs + SHA256-hashes every action
 ▼
Verification Layer
 │  - Binary Merkle tree over action log
 │  - EAS attestation on Ethereum Sepolia
 ▼
Partner Integrations
 ├── 0G Network  → decentralized agent memory (KV + append-only log)
 ├── Gensyn AXL  → agent-to-agent cross-node pub/sub messaging
 └── KeeperHub   → on-chain execution reliability + retry guarantees
```

---

## Stack

| Layer | Technology |
|---|---|
| Frontend | Next.js 15.5 · React 19 · TypeScript 5 · Tailwind CSS v4 |
| Web3 | RainbowKit v2 · wagmi v2 · viem v2 |
| Backend | Go 1.23 · chi · gorilla/websocket |
| Database | PostgreSQL 16 · pgx/v5 |
| Auth | SIWE nonce challenge · HS256 JWT |
| AI Agent | Anthropic Claude claude-opus-4-5 (structured tool use) |
| Containers | Docker Engine API · LUKS2 AES-256 encrypted volumes |
| Chain | **Ethereum Sepolia** (chainID 11155111) |
| Payments | x402 · EIP-3009 USDC `transferWithAuthorization` |
| Attestations | EAS — `0xC2679fBD37d54388Ce493F1DB75320D236e1815e` |
| Contracts | Hardhat 2.28 · Solidity 0.8.24 · OpenZeppelin v5 |
| Memory | 0G Network (decentralized KV + log) |
| Messaging | Gensyn AXL pub/sub |
| Reliability | KeeperHub on-chain job registry |

---

## Project Structure

```
comput3/
├── backend/                  # Go API server
│   ├── cmd/server/           # Entrypoint — wires all deps
│   ├── internal/
│   │   ├── agent/            # Claude agent loop (session.go) + 15 tools (tools.go)
│   │   ├── api/              # chi router, HTTP handlers, WebSocket stream
│   │   ├── auth/             # SIWE nonce + JWT service
│   │   ├── chain/            # RPC client, EAS, USDC, vault key, provider registry
│   │   ├── config/           # Env var loader
│   │   ├── container/        # Docker manager + LUKS2 encrypted volume setup
│   │   ├── scanner/          # GitHub repo analyzer (Claude)
│   │   └── store/            # PostgreSQL — migrations, all CRUD
│   └── integrations/
│       ├── zerog/            # 0G Network client (KV + append log)
│       ├── axl/              # Gensyn AXL pub/sub client
│       └── keeperhub/        # KeeperHub job registration client
│
├── contracts/                # Solidity smart contracts
│   ├── contracts/
│   │   ├── ProviderRegistry.sol   # Provider staking, registry, slashing
│   │   ├── DeploymentEscrow.sol   # Per-session ETH escrow + streaming release
│   │   └── JobAuction.sol         # Competitive provider bidding
│   ├── scripts/
│   │   ├── deploy.ts              # Deploy all 3 contracts → deployments.json
│   │   ├── register-eas-schema.ts # Register attestation schema on EAS
│   │   ├── export-abis.ts         # Copy ABIs to frontend/lib/contracts/
│   │   └── become-provider.ts     # Register wallet as compute provider
│   └── hardhat.config.ts          # ethSepolia network, Etherscan verification
│
├── frontend/                 # Next.js 15 app (standalone output)
│   ├── app/
│   │   ├── page.tsx               # Dashboard — containers + session stats
│   │   ├── deploy/                # Multi-phase deploy flow + live WS stream
│   │   ├── sessions/              # Session list + [sessionId] detail page
│   │   ├── attestations/          # EAS attestation list
│   │   ├── vault/                 # LUKS key retrieval (nonce → key)
│   │   ├── secrets/               # Encrypted secrets CRUD
│   │   ├── payments/              # x402 payment history + wallet funding
│   │   ├── audit/                 # Action log + Merkle proof viewer
│   │   ├── onboarding/            # Team name setup
│   │   ├── settings/              # Wallet + team info
│   │   └── provider/              # Provider dashboard, register, rentals, earnings, attestations
│   ├── components/
│   │   ├── Sidebar.tsx            # Navigation (user + provider modes)
│   │   ├── Web3Providers.tsx      # wagmi + RainbowKit + AuthContext wrapper
│   │   └── WalletButton.tsx       # Connect + Sign-in button
│   └── lib/
│       ├── api.ts                 # apiFetch helper, types
│       ├── AuthContext.tsx        # SIWE auth state + localStorage
│       ├── wagmi.ts               # wagmiConfig (Ethereum Sepolia)
│       ├── x402.ts                # EIP-712 typed data builder for x402 payments
│       └── contracts/typechain.ts # ProviderRegistry ABI + deployed addresses
│
├── integrations/             # Root-level integration stubs (docs/wiring)
│   ├── 0g/client.go
│   ├── axl/client.go
│   └── keeperhub/client.go
│
├── docs/
│   ├── architecture.md
│   ├── checklist.md
│   ├── dev-guidelines.md
│   └── implementation.md
│
├── scripts/
│   ├── deploy-contracts.sh        # Compile + deploy to Ethereum Sepolia
│   ├── register-provider.sh       # Register node in ProviderRegistry
│   └── register-eas-schema.sh     # Register EAS attestation schema
│
├── docker-compose.yml        # postgres:16 + docker:27-dind + backend + frontend
├── .env.example              # All env vars with descriptions
└── README.md
```

---

## Quick Start

### Prerequisites

- Go 1.23+
- Node.js 22+
- Docker + Docker Compose

### 1. Clone and configure

```bash
git clone https://github.com/comput3ai/comput3
cd comput3
cp .env.example .env
# Required: ANTHROPIC_API_KEY, AGENT_WALLET_PRIVATE_KEY, JWT_SECRET, VAULT_MASTER_SECRET
```

### 2. Start the local stack

```bash
# Postgres + Docker-in-Docker
docker compose up -d postgres dind

# Backend
cd backend && go run ./cmd/server

# Frontend (separate terminal)
cd frontend && cp .env.local.example .env.local && npm install && npm run dev
```

### 3. Deploy contracts to Ethereum Sepolia

```bash
# Requires DEPLOYER_PRIVATE_KEY and ETH_SEPOLIA_RPC_URL in .env
./scripts/deploy-contracts.sh

# Register EAS attestation schema
./scripts/register-eas-schema.sh

# Register your node as a provider
./scripts/register-provider.sh
```

### 4. Run everything with Docker

```bash
docker compose up --build
# frontend → http://localhost:3000
# backend  → http://localhost:8080
```

---

## API Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/health` | — | Liveness probe |
| POST | `/auth/nonce` | — | Issue SIWE challenge |
| POST | `/auth/verify` | — | Verify signature → JWT |
| GET | `/account?wallet=` | — | Get team by wallet |
| POST | `/account` | JWT | Update team name |
| POST | `/sessions` | JWT + x402 | Create agent session |
| GET | `/sessions/{id}` | JWT | Get session state |
| POST | `/sessions/{id}/confirm` | JWT | Confirm deployment plan |
| GET | `/sessions/{id}/audit` | JWT | Full action log + Merkle proofs |
| GET | `/sessions/{id}/stream` | JWT WS | WebSocket event stream |
| GET | `/providers/active` | — | Active providers from chain |
| GET | `/teams/{id}/sessions` | JWT | Team session list |
| GET | `/teams/{id}/attestations` | JWT | Team EAS attestations |
| GET | `/teams/{id}/workspaces` | JWT | Provisioned containers |
| GET | `/vault/nonce` | JWT | Vault key challenge nonce |
| POST | `/vault/key` | JWT | Derive container LUKS key |
| GET | `/payments` | JWT | Payment history |
| GET | `/secrets` | JWT | List secrets |
| POST | `/secrets` | JWT | Create secret |
| DELETE | `/secrets/{id}` | JWT | Delete secret |

---

## Smart Contracts (Ethereum Sepolia)

| Contract | Description |
|----------|-------------|
| `ProviderRegistry` | Provider registration with 0.01 ETH stake, slashing, jobs counter |
| `DeploymentEscrow` | Per-session ETH deposit with time-proportional streaming release |
| `JobAuction` | Competitive job bidding across registered providers |

Contract addresses are written to `contracts/deployments.json` after `deploy-contracts.sh` and should be copied to `.env`.

---

## Key Features

| Feature | How |
|---|---|
| **Trustless Execution** | Provider cannot read workload — LUKS2 AES-256 volume, key never on provider disk |
| **Verifiable Logs** | Every agent action: SHA256-hashed, binary Merkle tree, root attested on-chain via EAS |
| **Decentralized Providers** | On-chain `ProviderRegistry` with ETH stake, reputation, and slashable collateral |
| **AI Agent Orchestration** | Claude with 15 structured tools: analyze, plan, create container, exec, clone, write file, health check… |
| **x402 Micropayments** | USDC `transferWithAuthorization` (EIP-3009) — pay-per-session, no pre-approval, no escrow round-trip |
| **Decentralized Memory** | Agent state + action log persisted to 0G Network — survives node restarts |
| **Cross-Agent Messaging** | Gensyn AXL pub/sub — subtask delegation to specialized provider nodes |
| **Execution Reliability** | KeeperHub wraps attestation submission + escrow release as on-chain keeper jobs |

---

## Partner Integrations

### 0G Network — Decentralized Agent Memory
Every agent action is appended to a 0G log keyed by session ID. On reconnect the agent reloads its full history. Team KV data (workspace state, deployment metadata) is stored in 0G KV storage — no central database dependency for agent continuity.

### Gensyn AXL — Cross-Node Agent Communication
Multi-step deployments can delegate sub-tasks to specialized providers via AXL. The parent agent publishes to `comput3.session.<id>` — child agents subscribe, execute, and report back. Enables horizontal scaling of complex workloads across the provider network.

### KeeperHub — On-Chain Execution Reliability
EAS attestation submission and escrow release are registered as KeeperHub jobs after session completion. If a transaction fails or a node goes offline, KeeperHub automatically re-triggers the on-chain step — guaranteeing attestations are always submitted.

---

## Demo Flow

1. Connect wallet → sign in with SIWE
2. Submit a GitHub repo URL or task prompt
3. Agent analyzes the repo, queries on-chain providers, selects cheapest active node
4. Agent presents execution plan → user confirms in UI
5. Provider node creates a LUKS2-encrypted Docker container
6. Agent executes steps inside the container; each action is hashed and streamed live
7. On completion, a Merkle root of all actions is submitted as an EAS attestation on Ethereum Sepolia
8. User views the full verified audit log with Merkle proofs and accesses their live workspace

---

## Environment Variables

Key variables — see [`.env.example`](.env.example) for the full annotated list.

| Variable | Required | Description |
|---|---|---|
| `ANTHROPIC_API_KEY` | ✓ | Claude API key |
| `JWT_SECRET` | ✓ | 64-char hex, signs auth tokens |
| `VAULT_MASTER_SECRET` | ✓ | 64-char hex, HMAC key for LUKS key derivation |
| `AGENT_WALLET_PRIVATE_KEY` | ✓ | Backend wallet for EAS attestations + USDC transfers |
| `DEPLOYER_PRIVATE_KEY` | deploy | Deployer wallet for `deploy-contracts.sh` |
| `ETH_SEPOLIA_RPC_URL` | ✓ | Ethereum Sepolia RPC (default: `https://rpc.sepolia.org`) |
| `PROVIDER_REGISTRY_ADDRESS` | post-deploy | From `contracts/deployments.json` |
| `EAS_SCHEMA_UID` | post-deploy | From `register-eas-schema.sh` |
| `DATABASE_URL` | ✓ | PostgreSQL connection string |
| `NEXT_PUBLIC_WALLETCONNECT_PROJECT_ID` | frontend | WalletConnect cloud project ID |

---

## License

MIT
