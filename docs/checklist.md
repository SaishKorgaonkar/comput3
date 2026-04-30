# COMPUT3 — Implementation Checklist

Track progress across the full system. Each item should be marked when the feature is working end-to-end, not just scaffolded.
Legend: ✅ done · 🔧 pending · ⭕ optional/future

---

## Core System

### Agent Loop
- ✅ `Session` struct with state machine (running / completed / failed)
- ✅ Claude API integration with structured tool use
- ✅ Tool: `analyze_repo` → produces `DeploymentPlan`
- ✅ Tool: `generate_deployment_plan` → emits plan event, blocks on confirm
- ✅ Tool: `select_provider` → reads chain, picks cheapest active provider
- ✅ Tool: `create_container` → delegates to container manager
- ✅ Tool: `install_packages` → exec into container
- ✅ Tool: `configure_network` → shared Docker network for multi-container
- ✅ Tool: `run_command` / `start_process` / `write_file` / `clone_repo`
- ✅ WebSocket event stream: plan, action, message, done, error
- ✅ User confirmation gate (`confirmCh`)
- ✅ Action hash computation (SHA256 per step)
- 🔧 Session resumability (reload from 0G on reconnect) — blocked on real 0G client

### Provider Selection
- ✅ `chain.GetActiveProviders()` → raw `eth_call` to ProviderRegistry
- ✅ Provider sorting by price (ascending)
- ✅ Fallback to local node if no providers available
- 🔧 Provider endpoint health check before selection — HTTP ping not implemented

### Execution Layer (Docker)
- ✅ `container.Manager` initialized with Docker client
- ✅ `Create()` — pull image, set RAM/CPU limits, expose ports
- ✅ `Exec()` / `RunCommand()` / `StartProcess()` — run commands, capture stdout/stderr
- ✅ `Destroy()` with cleanup
- ✅ Port mapping registry (container → host port)
- ⭕ Subdomain proxy routing (`*.deploy.comput3.xyz → containerPort`)
- ⭕ SSH gateway for direct container terminal access

### LUKS Encryption
- ✅ `setupLUKSHome()` — create 512MB `vault.img`, format LUKS2, mount
- ✅ Vault key derivation: `HMAC-SHA256(masterSecret, containerID)`
- ✅ Temp keyfile written, used, deleted after mount
- ✅ Bind-mount encrypted volume into container at `/app`
- ✅ `teardownLUKSHome()` — umount, close mapper, detach loop device
- ✅ Vault key gate endpoint (`/vault/key`) with nonce verification

### Logging System
- ✅ `Action` struct: index, tool, input, result, error, timestamp, hash
- ✅ Append-only action log per session
- ✅ Action log persisted to DB (`action_logs` table) on session completion
- ✅ Action log streamed to 0G Network per action (NoopClient until real 0G wired)

### Hashing + Merkle Root
- ✅ SHA256 hash computed for each action at execution time
- ✅ Binary Merkle tree construction over `[]Action`
- ✅ Merkle root stored in `sessions` table on completion
- ✅ Merkle proof generation per action (`ComputeMerkleProof`)
- ✅ Audit endpoint: return full log + per-action proofs + DB fallback
- 🔧 Audit UI: render proof array and allow individual verification

---

## Blockchain

### ProviderRegistry Contract
- ✅ `register(endpoint, pricePerHour)` with 0.01 ETH stake
- ✅ `update(endpoint, pricePerHour)` — provider can update rate
- ✅ `getActiveProviders()` view function
- ✅ `deactivate()` / `reactivate()` by provider
- ✅ `slash(wallet, evidence)` by slash authority
- ✅ `recordJobCompleted(wallet)` by authorized backend
- ✅ Deployed on Ethereum Sepolia
- ✅ ABI exported to frontend
- 🔧 Backend calls `recordJobCompleted` after each session ends
- 🔧 Provider settings page (frontend) to call `update()` post-registration

### DeploymentEscrow Contract
- ✅ `startSession(sessionId, provider, ratePerSecond)` with ETH deposit
- ✅ `release(sessionId)` by release authority on completion
- ✅ `refund(sessionId)` by user after lockup
- ✅ Deployed on Ethereum Sepolia
- 🔧 Backend wires escrow `startSession` + `release` calls around session lifecycle

### EAS Attestation
- ✅ `chain.SubmitAttestation()` — builds, signs, submits `attest()` tx
- ✅ Attestation UID resolved from mined receipt (`WaitForAttestationUID`)
- ✅ `eas_scan_url` stored in DB and returned in attestation responses
- ✅ Attestation TX hash + UID stored in `attestations` table
- 🔧 Schema registered on Ethereum Sepolia EAS (needs `register-eas-schema.sh` run)
- 🔧 Attestation detail page shows EAS scan link

---

## Integrations

### 0G Memory
- ✅ Interface + NoopClient defined (`integrations/zerog/client.go`)
- ✅ Agent calls `Append` after each action (passes through Noop silently)
- 🔧 Real 0G client implementation (connect to 0G Flow contract + KV node)
- 🔧 Session state loaded from 0G on reconnect

### Gensyn AXL Communication
- ✅ Interface + NoopClient defined (`integrations/axl/client.go`)
- ✅ Topic convention: `comput3.session.{sessionID}`
- ✅ Message types defined: `task.assigned`, `task.status`, `task.complete`
- 🔧 Real AXL pub/sub client implementation
- ⭕ Multi-provider subtask delegation end-to-end

### KeeperHub Execution Wrapper
- ✅ Interface + NoopClient defined (`integrations/keeperhub/client.go`)
- ✅ `RegisterJob` called after session completes (attestation job)
- 🔧 Real KeeperHub HTTP client implementation
- 🔧 Escrow release wrapped as KeeperHub job

---

## API Layer

### Authentication
- ✅ POST `/auth/nonce` — issue SIWE challenge
- ✅ POST `/auth/verify` — verify signature, issue JWT
- ✅ JWT middleware protecting authenticated routes
- ✅ Wallet address + team ID extracted from JWT

### Session Endpoints
- ✅ POST `/sessions` — create session, x402 gated
- ✅ GET `/sessions/{id}` — return session state + metadata
- ✅ POST `/sessions/{id}/confirm` — user confirms plan
- ✅ GET `/sessions/{id}/stream` — WebSocket event stream
- ✅ GET `/sessions/{id}/audit` — full action log + Merkle proofs (in-memory + DB fallback)
- 🔧 POST `/sessions/{id}/attest` — manual attestation re-trigger

### Provider & Task Endpoints
- ✅ GET `/providers/active` — return active providers from chain
- ✅ GET `/health` — liveness probe

### Payment Endpoints
- ✅ x402 middleware on `/sessions` — 402 with public agent address
- ✅ GET `/payments` — payment history for wallet

### Workspace & Team Endpoints
- ✅ GET `/teams/{id}/workspaces` — list provisioned containers
- ✅ GET `/teams/{id}/sessions` — session list
- ✅ GET `/teams/{id}/attestations` — attestation list

### Secrets
- ✅ GET `/secrets` · POST `/secrets` · DELETE `/secrets/{id}`

---

## Frontend

### Core Pages
- ✅ `/deploy` — repo submit, scan, live WS stream, plan confirm
- ✅ `/sessions` — session list
- ✅ `/sessions/[id]` — session detail
- ✅ `/attestations` — EAS attestation list
- ✅ `/audit` — action log viewer
- ✅ `/vault` — LUKS key retrieval
- ✅ `/secrets` — encrypted secrets CRUD
- ✅ `/payments` — payment history
- ✅ `/provider/register` — register provider on-chain
- ✅ `/provider/page` — provider dashboard
- 🔧 `/audit` — render per-action Merkle proofs with verify button
- 🔧 `/provider/settings` — call `update()` to change endpoint/price post-registration
- 🔧 `/attestations/[id]` — link to EAS scan URL

---

## Demo Readiness

### End-to-End Flow
- ✅ Wallet connect + SIWE auth
- ✅ Submit GitHub repo URL → agent scans → plan presented
- ✅ User confirms → deployment streams live
- ✅ LUKS container created, packages installed, app started
- ✅ Session completes → Merkle root computed → EAS attestation submitted
- 🔧 Audit log with Merkle proofs visible and verifiable in UI
- 🔧 Provider `recordJobCompleted` incremented on-chain after session

### Infrastructure
- ✅ `docker-compose.yml` — postgres + docker-in-docker + backend + frontend
- ✅ Backend `Dockerfile`
- ✅ Frontend `Dockerfile`
- ✅ `scripts/deploy-contracts.sh`
- ✅ `scripts/register-provider.sh`
- ✅ `scripts/register-eas-schema.sh`

---

## Optional / Future

### JobAuction — On-Chain Competitive Bidding
> Contract is deployed and functional. Skipped in current demo flow (agent reads registry directly). Implement when multi-provider network exists.

- ✅ `JobAuction.sol` deployed — `postJob()`, `submitBid()`, `closeAuction()`, 30s bid window
- ⭕ Agent calls `postJob()` instead of `select_provider` direct read
- ⭕ Provider backend watches `JobPosted` events and calls `submitBid()`
- ⭕ `closeAuction()` triggers `DeploymentEscrow.startSession()` automatically
- ⭕ Fallback to single registered provider if no bids within window

### AXL Multi-Agent Delegation
- ⭕ Real AXL client wired
- ⭕ User agent publishes subtask to `comput3.session.<id>`
- ⭕ Provider agent subscribes, executes, reports back
- ⭕ End-to-end multi-provider parallel execution

### Subdomain Proxy + SSH Gateway
- ⭕ `*.deploy.comput3.xyz` reverse proxy to container ports
- ⭕ SSH gateway for direct terminal access to workspace containers
