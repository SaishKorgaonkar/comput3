# COMPUT3

**Trustless decentralized compute for AI agents — private by default, verifiable by design.**

---

## Problem

AI agents need to execute code, run workloads, and process sensitive data — but today they rely entirely on centralized infrastructure. There is no way to verify what actually ran, providers can read user data, and there is no economic accountability for compute providers.

## Solution

COMPUT3 is a decentralized compute network where:

- Users submit tasks to an AI agent (Claude claude-opus-4-5)
- The agent selects a provider node from an on-chain registry (Base Sepolia)
- Execution happens inside an **encrypted container** (LUKS2 AES-256) — the provider cannot read user data
- Every action is **logged, hashed, and Merkle-verified**
- A Merkle root of execution is submitted as an **EAS attestation** on Base Sepolia
- Providers stake ETH as collateral and can be slashed for misbehavior
- Payments are streamed as USDC micro-payments via the **x402 protocol**

---

## Architecture

```
User
 │
 ▼
AI Agent (Claude)
 │  - analyze task
 │  - select provider (on-chain)
 │  - plan execution
 ▼
Provider Node (COMPUT3 backend)
 │  - spins Docker container
 │  - mounts LUKS2 encrypted volume
 │  - executes steps
 │  - logs + hashes every action
 ▼
Verification Layer
 │  - Merkle root of action log
 │  - EAS attestation on Base Sepolia
 ▼
Partner Integrations
 ├── 0G       → decentralized agent memory (KV store / logs)
 ├── Gensyn AXL → agent-to-agent cross-node messaging
 └── KeeperHub  → execution reliability + retry guarantees
```

---

## Stack

| Layer        | Technology |
|--------------|------------|
| Frontend     | Next.js 15 · React 19 · TypeScript 5 · Tailwind CSS v4 |
| Web3         | RainbowKit v2 · wagmi v2 · viem v2 |
| Backend      | Go 1.23 · chi router · gorilla/websocket |
| Database     | PostgreSQL 16 via pgx/v5 |
| Auth         | SIWE nonce + HS256 JWT |
| AI Agent     | Anthropic Claude (claude-opus-4-5) |
| Containers   | Docker Engine + LUKS2 encrypted volumes |
| Chain        | Base Sepolia (chainID 84532) |
| Payments     | x402 · EIP-3009 USDC transferWithAuthorization |
| Attestations | EAS (Ethereum Attestation Service) |
| Contracts    | Hardhat 2 · Solidity 0.8.24 · OpenZeppelin v5 |

---

## Project Structure

```
comput3/
├── backend/                  # Go API server
│   ├── cmd/server/           # Entry point
│   ├── internal/
│   │   ├── agent/            # Claude agent loop + tools
│   │   ├── api/              # HTTP handlers + WebSocket stream
│   │   ├── auth/             # SIWE nonce + JWT
│   │   ├── chain/            # RPC, EAS, USDC, vault key, provider
│   │   ├── config/           # Environment config
│   │   ├── container/        # Docker manager + LUKS encryption
│   │   ├── scanner/          # GitHub repo analyzer
│   │   └── store/            # PostgreSQL data layer
│   └── integrations/
│       ├── zerog/            # 0G Network client
│       ├── axl/              # Gensyn AXL pub/sub client
│       └── keeperhub/        # KeeperHub execution wrapper
│
├── contracts/                # Solidity smart contracts (Hardhat)
│   ├── contracts/
│   │   ├── ProviderRegistry.sol
│   │   ├── DeploymentEscrow.sol
│   │   └── JobAuction.sol
│   └── scripts/
│       ├── deploy.ts
│       ├── register-eas-schema.ts
│       └── become-provider.ts
│
├── frontend/                 # Next.js 15 app
│   ├── app/
│   │   ├── page.tsx          # Dashboard
│   │   ├── deploy/           # Deploy form (WebSocket stream)
│   │   ├── sessions/         # Sessions list + detail
│   │   ├── attestations/     # EAS attestations
│   │   ├── vault/            # LUKS key retrieval
│   │   ├── secrets/          # Encrypted secrets CRUD
│   │   ├── payments/         # x402 payment history
│   │   ├── audit/            # Action log + Merkle proofs
│   │   ├── onboarding/       # Team setup
│   │   ├── settings/         # Wallet + team info
│   │   └── provider/         # Provider dashboard, register, rentals, earnings
│   └── lib/
│       ├── api.ts            # API client
│       ├── AuthContext.tsx   # Wallet auth state
│       ├── wagmi.ts          # wagmi config
│       ├── x402.ts           # x402 payment builder
│       └── contracts/        # ABI + contract addresses
│
├── docs/
│   ├── architecture.md
│   ├── checklist.md
│   ├── dev-guidelines.md
│   └── implementation.md
│
├── scripts/
│   ├── deploy-contracts.sh
│   ├── register-provider.sh
│   └── register-eas-schema.sh
│
├── docker-compose.yml        # Local dev stack
├── .env.example              # All required env vars
└── README.md
```

---

## Quick Start

### Prerequisites
- Go 1.23+
- Node.js 22+
- Docker + Docker Compose
- PostgreSQL 16 (or use the compose stack)

### 1. Clone and configure

```bash
git clone https://github.com/comput3ai/comput3
cd comput3
cp .env.example .env
# Fill in your ANTHROPIC_API_KEY and wallet keys
```

### 2. Start the dev stack

```bash
# Start Postgres + Docker-in-Docker
docker compose up -d postgres dind

# Start backend (from backend/)
cd backend && go run ./cmd/server

# Start frontend (from frontend/)
cd frontend && npm install && npm run dev
```

### 3. Deploy contracts

```bash
./scripts/deploy-contracts.sh
```

### 4. Register a provider node

```bash
./scripts/register-provider.sh
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
| GET | `/sessions/{id}/audit` | JWT | Full action log + Merkle |
| GET | `/sessions/{id}/stream` | JWT | WebSocket event stream |
| GET | `/providers/active` | — | Active providers from chain |
| GET | `/teams/{id}/sessions` | JWT | Team session list |
| GET | `/teams/{id}/attestations` | JWT | Team attestations |
| GET | `/teams/{id}/workspaces` | JWT | Provisioned containers |
| GET | `/vault/nonce` | JWT | Vault key challenge nonce |
| POST | `/vault/key` | JWT | Derive container LUKS key |
| GET | `/payments` | JWT | Payment history |
| GET | `/secrets` | JWT | List secrets |
| POST | `/secrets` | JWT | Create secret |
| DELETE | `/secrets/{id}` | JWT | Delete secret |

---

## Smart Contracts (Base Sepolia)

| Contract | Description |
|----------|-------------|
| `ProviderRegistry` | Provider registration with 0.01 ETH stake |
| `DeploymentEscrow` | Per-session ETH deposit with streaming release |
| `JobAuction` | Competitive job bidding for providers |

---

## Environment Variables

See [`.env.example`](.env.example) for the full list with descriptions.


---

## Key Features

| Feature | Description |
|---|---|
| **Trustless Execution** | Providers cannot read workload data; execution is isolated |
| **Encrypted Compute** | LUKS2 AES-256 encrypted volumes per container; key is blockchain-gated |
| **Verifiable Logs** | Every action is SHA256-hashed; Merkle root submitted on-chain via EAS |
| **Decentralized Providers** | On-chain ProviderRegistry with staking, reputation, and slashing |
| **AI Agent Orchestration** | Claude-based agent with structured tools for deployment and task execution |
| **x402 Micropayments** | USDC `transferWithAuthorization` — pay-per-task with no pre-approval |

---

## Partner Integrations

### 0G — Decentralized Memory
Agent state, task history, and KV data are stored on 0G's decentralized storage network. This means agent memory survives node restarts and is not controlled by any single operator.

### Gensyn AXL — Agent Communication
When tasks require coordination across multiple provider nodes, agents communicate using the AXL messaging protocol. This enables multi-agent workflows where subtasks are routed to specialized nodes.

### KeeperHub — Execution Reliability
Critical execution steps (container creation, attestation submission) are wrapped with KeeperHub's on-chain retry guarantees. If a step fails, KeeperHub automatically re-triggers it without user intervention.

---

## Demo Flow

1. User connects wallet and signs in (SIWE)
2. User submits a task prompt (e.g. "Deploy this GitHub repo")
3. Agent analyzes the task, selects a provider from the on-chain registry
4. Agent presents an execution plan — user confirms
5. Provider node creates an encrypted Docker container
6. Agent executes steps inside the container, logging each action
7. On completion, a Merkle root of all actions is submitted as an EAS attestation
8. User can view the verified execution log and access their workspace

---

## Tech Stack

| Layer | Technology |
|---|---|
| Backend | Go 1.24, Chi, pgx, Docker SDK |
| Smart Contracts | Solidity 0.8.24, Hardhat, OpenZeppelin |
| Frontend | Next.js 14, wagmi v2, viem, Tailwind |
| AI Agent | Anthropic Claude (structured tool use) |
| Database | PostgreSQL |
| Encryption | LUKS2 (cryptsetup) |
| Chain | Base Sepolia |
| Payments | x402 (USDC transferWithAuthorization EIP-3009) |
| Attestations | EAS (Ethereum Attestation Service) |
| Memory | 0G Network |
| Messaging | Gensyn AXL |
| Reliability | KeeperHub |

---

## Setup

See [docs/implementation.md](docs/implementation.md) for module setup.

```bash
# Backend
cd backend && go mod tidy && go run ./cmd/server

# Contracts
cd contracts && npm install && npx hardhat compile

# Frontend
cd frontend && npm install && npm run dev
```

Environment variables: copy `.env.example` to `.env` in each subdirectory.

---

## Repository Structure

```
comput3/
├── backend/          Go API server, agent loop, container manager
├── contracts/        Solidity contracts (ProviderRegistry, EAS)
├── frontend/         Next.js user interface
├── integrations/
│   ├── 0g/           0G memory integration
│   ├── axl/          Gensyn AXL messaging
│   └── keeperhub/    KeeperHub reliability wrapper
├── docs/             Architecture, implementation, guidelines
└── scripts/          Deployment and utility scripts
```

---

## License

MIT
