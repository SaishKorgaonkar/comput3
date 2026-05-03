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
- ✅ Session resumability — completed sessions replayed from 0G log on WebSocket reconnect

### Provider Selection
- ✅ `chain.GetActiveProviders()` → raw `eth_call` to ProviderRegistry
- ✅ Provider sorting by price (ascending)
- ✅ Fallback to local node if no providers available
- ✅ Provider endpoint health check before selection — 3s GET /health ping

### Execution Layer (Docker)
- ✅ `container.Manager` initialized with Docker client
- ✅ `Create()` — pull image, set RAM/CPU limits, expose ports
- ✅ `Exec()` / `RunCommand()` / `StartProcess()` — run commands, capture stdout/stderr
- ✅ `Destroy()` with cleanup
- ✅ Port mapping registry (container → host port)
- ✅ Subdomain proxy routing — `subdomainProxyMiddleware` reverse-proxies `<containerID>.DEPLOY_DOMAIN` to container port
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
- ✅ Audit UI: render proof array with L/R sibling hashes per action

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
- ✅ Backend calls `recordJobCompleted` after each session ends
- ✅ Provider settings page (frontend) — calls `update()` at `/provider/settings`

### DeploymentEscrow Contract
- ✅ `startSession(sessionId, provider, ratePerSecond)` with ETH deposit
- ✅ `release(sessionId)` by release authority on completion
- ✅ `refund(sessionId)` by user after lockup
- ✅ Deployed on Ethereum Sepolia
- ✅ Backend calls `release(sessionId)` after session completes (`chain.ReleaseEscrow`)
- ✅ Frontend escrow deposit — optional "Fund Escrow" card in done panel calls `DeploymentEscrow.deposit()` via wagmi

### EAS Attestation
- ✅ `chain.SubmitAttestation()` — builds, signs, submits `attest()` tx
- ✅ Attestation UID resolved from mined receipt (`WaitForAttestationUID`)
- ✅ `eas_scan_url` stored in DB and returned in attestation responses
- ✅ Attestation TX hash + UID stored in `attestations` table
- ⚠️ Schema registration on Ethereum Sepolia EAS — **run `bash scripts/register-eas-schema.sh`**, then set `EAS_SCHEMA_UID` in `.env`
- ✅ `POST /sessions/{id}/attest` manual re-trigger endpoint implemented
- ✅ Attestation detail page `/attestations/[id]` — EAS scan link, TX hash, merkle root, action log with proofs

---

## Integrations

### 0G Memory
- ✅ Interface + NoopClient defined (`integrations/zerog/client.go`)
- ✅ Agent calls `Append` after each action (passes through Noop silently)
- ✅ Real 0G HTTP client implemented (storage node KV API at configured endpoint)
- ✅ Session state loaded from 0G on reconnect — `handleStreamSession` replays log for completed sessions
- ⚠️ Requires `ZG_RPC_URL` + `ZG_PRIVATE_KEY` + `ZG_FLOW_ADDRESS` in `.env` (falls back to NoopClient)

### Gensyn AXL Communication
- ✅ Interface + NoopClient defined (`integrations/axl/client.go`)
- ✅ Topic convention: `comput3.session.{sessionID}`
- ✅ Message types defined: `task.assigned`, `task.status`, `task.complete`
- ✅ Real AXL P2P client — `POST /send` + polling `GET /recv` via local AXL node HTTP API, topic envelope filtering
- ⚠️ Requires running AXL node; set `AXL_ENDPOINT` (e.g. `http://127.0.0.1:9002`) + `AXL_PEER_ID` in `.env`
- ⭕ Multi-provider subtask delegation end-to-end

### KeeperHub Execution Wrapper
- ✅ Interface + NoopClient defined (`integrations/keeperhub/client.go`)
- ✅ `RegisterJob` called after session completes (attestation job)
- ✅ Real KeeperHub HTTP client with HMAC-SHA256 auth (`POST /api/v1/jobs`)
- ✅ Escrow release wrapped as KeeperHub job — `release-escrow` job registered after session completes
- ✅ `POST /sessions/{id}/release-escrow` endpoint — KeeperHub workflow calls this to trigger on-chain release
- ⚠️ Requires `KEEPERHUB_ENDPOINT` + `KEEPERHUB_PRIVATE_KEY` in `.env`; configure KeeperHub workflow to call `/sessions/{id}/release-escrow`

### Projects + Environment Variables
- ✅ `projects` table — `id, team_id, name, repo_url, branch, last_prompt, webhook_secret, auto_deploy, last_deployed_at`
- ✅ `project_env_vars` table — `id, project_id, team_id, key, value (AES-GCM encrypted), UNIQUE(project_id, key)`
- ✅ GET `/projects` · POST `/projects` · GET `/projects/{id}` · PUT `/projects/{id}` · DELETE `/projects/{id}`
- ✅ GET `/projects/{id}/env` — list env var keys (no values)
- ✅ POST `/projects/{id}/env` — upsert encrypted env var
- ✅ DELETE `/projects/{id}/env/{envId}` — remove env var
- ✅ Webhook secret auto-generated per project on creation (returned once, never again)
- ✅ POST `/sessions` accepts `project_id` + `env_vars` — merges project env + ad-hoc vars
- ✅ Projects list UI page `/projects` — full CRUD, env var management, webhook URL display, Deploy button

### CI/CD Webhooks
- ✅ `POST /webhooks/github/{projectId}` — public endpoint, HMAC-SHA256 verified
- ✅ Branch filtering — only triggers on push to configured `branch`
- ✅ Auto-redeploy with `last_prompt` + decrypted project env vars
- ✅ `auto_deploy` flag on project — webhook ignored if false
- ✅ `last_deployed_at` updated via `TouchProjectDeployedAt` after CI redeploy
- ✅ Webhook setup UI in done panel — shows webhook URL + GitHub Actions setup instructions when `project_id` set
- ✅ GitHub Actions CI workflow — `.github/workflows/deploy.yml` triggers on push to main, HMAC-signs payload

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
- ✅ POST `/sessions/{id}/attest` — manual attestation re-trigger

### Provider & Task Endpoints
- ✅ GET `/providers/active` — return active providers from chain
- ✅ GET `/health` — liveness probe

### Payment Endpoints
- ✅ x402 middleware on `/sessions` — returns standard x402 `accepts` array with EIP-3009 USDC payment details
- ✅ Frontend x402 flow — deploy page handles 402, prompts MetaMask EIP-712 signature, retries with `X-Payment` header
- ✅ GET `/payments` — payment history for wallet

### Workspace & Team Endpoints
- ✅ GET `/teams/{id}/workspaces` — list provisioned containers
- ✅ GET `/teams/{id}/sessions` — session list
- ✅ GET `/teams/{id}/attestations` — attestation list

### Secrets
- ✅ GET `/secrets` · POST `/secrets` · DELETE `/secrets/{id}`
- ✅ Values encrypted at rest with AES-256-GCM (`VAULT_MASTER_SECRET` → SHA-256 → AES key)
- ✅ Legacy plaintext values handled gracefully (passthrough on decrypt failure)
- ✅ `/secrets` frontend — encrypted secrets CRUD

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
- ✅ `/audit` — render per-action Merkle proofs (L/R sibling hashes)
- ✅ `/provider/settings` — call `update()` to change endpoint/price post-registration
- ✅ `/attestations/[id]` — detail page with EAS scan link, TX hash, merkle root, schema UID, action log

### Frontend
- ✅ `/deploy` — env vars phase inserted between pick and prompt
  - Key/value editor with masked value input
  - Common variable suggestions (DATABASE_URL, REDIS_URL, etc.)
  - Env var count shown in pipeline stage sidebar
  - "Skip" option for projects without env vars
- ✅ `/deploy` — env var count shown in prompt confirmation banner
- ✅ Env vars passed to `POST /sessions` as `env_vars` map
- ✅ `/projects` — project list, create project, env var management, webhook URL, auto-deploy toggle
- ✅ Done panel — shows GitHub webhook URL + CI/CD instructions when deployed as a project

---

### End-to-End Flow
- ✅ Wallet connect + SIWE auth
- ✅ Submit GitHub repo URL → agent scans → plan presented
- ✅ User confirms → deployment streams live
- ✅ LUKS container created, packages installed, app started
- ✅ Session completes → Merkle root computed → EAS attestation submitted
- ✅ Audit log with per-action Merkle proofs visible in UI
- ✅ Provider `recordJobCompleted` incremented on-chain after session

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

- ✅ `JobAuction.sol` deployed — `postJob()`, `submitBid()`, `closeAuction()`, 30s bid window
- ✅ `chain/auction.go` — `PostJob`, `SubmitBid`, `CloseAuction`, `WatchJobAwarded`, `PollJobPostedEvents`
- ✅ Agent uses `selectProviderViaAuction` — posts job, waits 30s, closes, resolves winner
- ✅ Provider backend `StartProviderBidder` — polls `JobPosted` events, auto-submits bids
- ✅ `POST /providers/bid` endpoint for manual bid submission
- ✅ Fallback to registry direct-select if auction yields no winner

### AXL Multi-Agent Delegation
- ✅ Real AXL P2P client implemented
- ⭕ User agent publishes subtask to `comput3.session.<id>`
- ⭕ Provider agent subscribes, executes, reports back
- ⭕ End-to-end multi-provider parallel execution

### Subdomain Proxy + SSH Gateway
- ✅ `*.deploy.comput3.xyz` reverse proxy — `subdomainProxyMiddleware` in backend; activate with `DEPLOY_DOMAIN` env var + wildcard DNS
- ⭕ SSH gateway for direct terminal access to workspace containers

---

## Operational Checklist (run once before launch)

- ⚠️ Deploy contracts: `bash scripts/deploy-contracts.sh` → set `DEPLOYMENT_ESCROW_ADDRESS`, `PROVIDER_REGISTRY_ADDRESS`, `JOB_AUCTION_ADDRESS` in `.env`
- ⚠️ Register EAS schema: `bash scripts/register-eas-schema.sh` → set `EAS_SCHEMA_UID` in `.env`
- ⚠️ Register provider: `bash scripts/register-provider.sh` (if running as a provider node)
- ⚠️ Set `DEPLOY_DOMAIN=deploy.comput3.xyz` + add wildcard DNS `A` record `*.deploy.comput3.xyz → server IP`
- ⚠️ Set `AXL_ENDPOINT` + `AXL_PEER_ID` (run AXL node: `git clone https://github.com/gensyn-ai/axl && go build -o node ./cmd/node/ && ./node -config node-config.json`)
- ⚠️ Set `KEEPERHUB_ENDPOINT=https://api.keeperhub.dev` + `KEEPERHUB_PRIVATE_KEY` (org API key from app.keeperhub.dev)
- ⚠️ Set `ZG_RPC_URL` + `ZG_PRIVATE_KEY` + `ZG_FLOW_ADDRESS` (0G Network node access)
- ⚠️ Add GitHub repo secrets `COMPUT3_WEBHOOK_URL` + `COMPUT3_WEBHOOK_SECRET` for CI/CD
- ⚠️ Set frontend `NEXT_PUBLIC_DEPLOYMENT_ESCROW_ADDRESS`, `NEXT_PUBLIC_PROVIDER_REGISTRY_ADDRESS`, `NEXT_PUBLIC_JOB_AUCTION_ADDRESS`
