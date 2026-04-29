# COMPUT3 — Implementation Checklist

Track progress across the full system. Each item should be marked when the feature is working end-to-end, not just scaffolded.

---

## Core System

### Agent Loop
- [ ] `Session` struct with state machine (running / completed / failed)
- [ ] Claude API integration with structured tool use
- [ ] Tool: `analyze_repo` → produces `DeploymentPlan`
- [ ] Tool: `generate_deployment_plan` → emits plan event, blocks on confirm
- [ ] Tool: `select_provider` → reads chain, picks cheapest active provider
- [ ] Tool: `create_container` → delegates to container manager
- [ ] Tool: `install_packages` → exec into container
- [ ] Tool: `configure_network` → shared Docker network for multi-container
- [ ] Tool: `exec_command` → arbitrary command in container
- [ ] WebSocket event stream: plan, action, message, done, error
- [ ] User confirmation gate (`confirmCh`)
- [ ] Action hash computation (SHA256 per step)
- [ ] Session resumability (reload from 0G on reconnect)

### Provider Selection
- [ ] `chain.GetActiveProviders()` → raw `eth_call` to ProviderRegistry
- [ ] Provider sorting by price (ascending)
- [ ] Fallback to COMPUT3 node if no providers available
- [ ] Provider endpoint health check before selection

### Execution Layer (Docker)
- [ ] `container.Manager` initialized with Docker client
- [ ] `Create()` — pull image, set RAM/CPU limits, expose ports
- [ ] `Exec()` — run command inside container, capture stdout/stderr
- [ ] `Stop()` and `Remove()` with cleanup
- [ ] Port mapping registry (container → host port)
- [ ] Subdomain proxy routing (`*.deploy.comput3.xyz → containerPort`)
- [ ] Workspace provisioning (VS Code Server or Jupyter)
- [ ] SSH gateway for direct container terminal access

### LUKS Encryption
- [ ] `setupLUKSHome()` — create 512MB `vault.img`, format LUKS2, mount
- [ ] Vault key derivation: `HMAC-SHA256(masterSecret, containerID)`
- [ ] Temp keyfile written, used, deleted after mount
- [ ] Bind-mount encrypted volume into container at `/app`
- [ ] `teardownLUKSHome()` — umount, close mapper, detach loop device
- [ ] Vault key gate endpoint (`/vault/key`) with blockchain signature verification

### Logging System
- [ ] `Action` struct: index, tool, input, result, error, timestamp, hash
- [ ] Append-only action log per session
- [ ] Structured JSON logging to stdout
- [ ] Action log persisted to DB (`action_logs` table)
- [ ] Action log streamed to 0G Network per action

### Hashing + Merkle Root
- [ ] SHA256 hash computed for each action at execution time
- [ ] Binary Merkle tree construction over `[]Action`
- [ ] Merkle root stored in `sessions` table on completion
- [ ] Merkle proof generation for individual actions
- [ ] Audit endpoint: return full log + proofs

---

## Blockchain

### ProviderRegistry Contract
- [ ] `register(endpoint, pricePerHour)` with 0.01 ETH stake
- [ ] `getActiveProviders()` view function
- [ ] `deactivate()` / `reactivate()` by provider
- [ ] `slash(wallet, evidence)` by slash authority
- [ ] `incrementJobsCompleted(wallet)` by authorized backend
- [ ] Deployed and verified on Base Sepolia
- [ ] ABI exported for backend

### DeploymentEscrow Contract
- [ ] `startSession(sessionId, provider, ratePerSecond)` with ETH deposit
- [ ] Streaming payment: `drip()` releases wei proportional to elapsed time
- [ ] `release(sessionId)` by release authority on completion
- [ ] `refund(sessionId)` by user after 24hr lockup
- [ ] `slash()` integration with ProviderRegistry
- [ ] Deployed and verified on Base Sepolia

### EAS Attestation
- [ ] Schema registered on Base Sepolia EAS
- [ ] `chain.SubmitAttestation()` — builds and signs `attest()` tx
- [ ] Session `merkleRoot`, `sessionID`, `teamID`, `providerAddr` encoded in attestation data
- [ ] Attestation TX hash stored in DB
- [ ] Attestation viewable via `/attestations/{sessionID}`

---

## Integrations

### 0G Memory
- [ ] `integrations/0g/client.go` — 0G SDK wrapper initialized
- [ ] `Put` / `Get` for session state
- [ ] `Append` / `ReadLog` for append-only action log
- [ ] Called from agent: store plan after analyze_repo
- [ ] Called from agent: append action after each tool call
- [ ] Session state loaded from 0G on reconnect
- [ ] Configuration: `0G_RPC_URL`, `0G_PRIVATE_KEY`, `0G_FLOW_ADDRESS`

### Gensyn AXL Communication
- [ ] `integrations/axl/client.go` — AXL client initialized
- [ ] `Publish(topic, message)` implemented
- [ ] `Subscribe(topic, handler)` implemented
- [ ] Topic convention: `comput3.session.{sessionID}`
- [ ] Message types: `task.assigned`, `task.status`, `task.complete`
- [ ] Parent agent delegates subtask via AXL
- [ ] Child agent receives, executes, reports back
- [ ] Multi-provider scenario working end-to-end

### KeeperHub Execution Wrapper
- [ ] `integrations/keeperhub/client.go` — KeeperHub client initialized
- [ ] `RegisterJob(job)` implemented
- [ ] `CancelJob(jobID)` implemented
- [ ] EAS attestation submission wrapped as KeeperHub job
- [ ] `jobsCompleted` increment wrapped as KeeperHub job
- [ ] Escrow release wrapped as KeeperHub job
- [ ] Job status observable in admin dashboard

---

## API Layer

### Authentication
- [ ] POST `/auth/nonce` — issue SIWE challenge
- [ ] POST `/auth/verify` — verify signature, issue JWT
- [ ] JWT middleware protecting authenticated routes
- [ ] Wallet address extracted from JWT for all requests

### Session Endpoints
- [ ] POST `/sessions` — create session, x402 gated
- [ ] GET `/sessions/{id}` — return session state + metadata
- [ ] POST `/sessions/{id}/confirm` — user confirms plan
- [ ] GET `/sessions/{id}/stream` — WebSocket event stream
- [ ] GET `/sessions/{id}/audit` — full action log + Merkle proofs
- [ ] POST `/sessions/{id}/attest` — manual attestation trigger

### Provider & Task Endpoints
- [ ] GET `/providers/active` — return active providers from chain
- [ ] GET `/health` — liveness probe

### Payment Endpoints
- [ ] x402 middleware on compute routes
- [ ] GET `/payments` — payment history for wallet

### Workspace Endpoints
- [ ] GET `/teams/{id}/workspaces` — list provisioned workspaces
- [ ] Subdomain proxy: `*.deploy.comput3.xyz → container port`

### Secrets
- [ ] GET `/secrets` — list encrypted secrets for wallet
- [ ] POST `/secrets` — store encrypted secret
- [ ] DELETE `/secrets/{id}`

---

## Demo

### End-to-End Flow
- [ ] User can connect wallet and authenticate
- [ ] User can submit a GitHub repo URL
- [ ] Agent analyzes repo and presents plan in UI
- [ ] User confirms plan and sees live execution stream
- [ ] Container is created with LUKS encryption
- [ ] Packages installed, app deployed
- [ ] Session completes, attestation submitted on Base Sepolia
- [ ] User can view audit log and Merkle proof in UI

### Multi-Provider Setup
- [ ] At least 2 provider nodes registered on-chain
- [ ] Provider selection chooses cheapest
- [ ] Multi-agent subtask delegation via AXL (bonus)

### Verification Visible
- [ ] Action log with hashes visible in UI
- [ ] Merkle root displayed on session completion
- [ ] EAS attestation TX linked to Base Sepolia explorer
- [ ] Individual action Merkle proof verifiable

---

## Infrastructure

- [ ] Docker Compose for local development (backend + postgres + docker-in-docker)
- [ ] `.env.example` files for backend, frontend, contracts
- [ ] Backend `Dockerfile`
- [ ] Frontend `Dockerfile`
- [ ] `scripts/deploy-contracts.sh` — deploy + export ABIs
- [ ] `scripts/register-provider.sh` — register node in ProviderRegistry
