# COMPUT3

**Trustless decentralized compute for AI agents — private by default, verifiable by design.**

---

## Problem

AI agents need to execute code, run workloads, and process sensitive data — but today they rely entirely on centralized infrastructure. There is no way to verify what actually ran, providers can read user data, and there is no economic accountability for compute providers.

## Solution

COMPUT3 is a decentralized compute network where:

- Users submit tasks to an AI agent
- The agent selects a provider node from an on-chain registry
- Execution happens inside an **encrypted container** (LUKS2 AES-256) — the provider cannot read user data
- Every action is **logged, hashed, and Merkle-verified**
- A Merkle root of execution is submitted as an **EAS attestation** on Base Sepolia
- Providers stake ETH as collateral and can be slashed for misbehavior

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
 ├── 0G  → decentralized agent memory (KV store / logs)
 ├── Gensyn AXL → agent-to-agent cross-node messaging
 └── KeeperHub → execution reliability + retry guarantees
```

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
