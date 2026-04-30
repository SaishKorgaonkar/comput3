package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/websocket"

	"github.com/comput3ai/comput3/backend/integrations/keeperhub"
	"github.com/comput3ai/comput3/backend/internal/agent"
	"github.com/comput3ai/comput3/backend/internal/auth"
	"github.com/comput3ai/comput3/backend/internal/chain"
	"github.com/comput3ai/comput3/backend/internal/config"
	"github.com/comput3ai/comput3/backend/internal/container"
	"github.com/comput3ai/comput3/backend/internal/scanner"
	"github.com/comput3ai/comput3/backend/internal/store"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Server holds all dependencies for the HTTP API.
type Server struct {
	cfg        *config.Config
	db         *store.Store
	mgr        *container.Manager
	sc         *scanner.Scanner
	authSvc    *auth.Service
	keeper     keeperhub.Client
	zeroG      agent.ZeroGClient

	mu       sync.RWMutex
	sessions map[string]*agent.Session
}

// NewServer creates a new API server with all dependencies wired.
func NewServer(
	cfg *config.Config,
	db *store.Store,
	mgr *container.Manager,
	sc *scanner.Scanner,
	authSvc *auth.Service,
	keeper keeperhub.Client,
	zeroG agent.ZeroGClient,
) *Server {
	return &Server{
		cfg:      cfg,
		db:       db,
		mgr:      mgr,
		sc:       sc,
		authSvc:  authSvc,
		keeper:   keeper,
		zeroG:    zeroG,
		sessions: make(map[string]*agent.Session),
	}
}

// Router builds and returns the chi router.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware)

	r.Get("/health", s.handleHealth)

	// Auth
	r.Post("/auth/nonce", s.handleAuthNonce)
	r.Post("/auth/verify", s.handleAuthVerify)

	// Account (wallet-based, no JWT required for GET — wallet in query param)
	r.Get("/account", s.handleGetAccount)
	r.Group(func(r chi.Router) {
		r.Use(s.jwtMiddleware)
		r.Post("/account", s.handleUpdateAccount)
	})

	// Sessions
	r.Group(func(r chi.Router) {
		r.Use(s.jwtMiddleware)
		r.Post("/sessions", s.x402Middleware(s.handleCreateSession))
		r.Get("/sessions/{id}", s.handleGetSession)
		r.Post("/sessions/{id}/confirm", s.handleConfirmSession)
		r.Get("/sessions/{id}/audit", s.handleGetAudit)
		r.Get("/sessions/{id}/stream", s.handleStreamSession)
	})

	// Provider discovery
	r.Get("/providers/active", s.handleGetProviders)

	// Team resources (JWT required)
	r.Group(func(r chi.Router) {
		r.Use(s.jwtMiddleware)
		r.Get("/teams/{id}/workspaces", s.handleListWorkspaces)
		r.Get("/teams/{id}/sessions", s.handleListTeamSessions)
		r.Get("/teams/{id}/attestations", s.handleListTeamAttestations)
	})

	// Vault
	r.Group(func(r chi.Router) {
		r.Use(s.jwtMiddleware)
		r.Get("/vault/nonce", s.handleVaultNonce)
		r.Post("/vault/key", s.handleVaultKey)
	})

	// Payments
	r.Group(func(r chi.Router) {
		r.Use(s.jwtMiddleware)
		r.Get("/payments", s.handleListPayments)
	})

	// Secrets
	r.Group(func(r chi.Router) {
		r.Use(s.jwtMiddleware)
		r.Get("/secrets", s.handleListSecrets)
		r.Post("/secrets", s.handleCreateSecret)
		r.Delete("/secrets/{id}", s.handleDeleteSecret)
	})

	return r
}

// --- Auth ---

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "ok"})
}

func (s *Server) handleAuthNonce(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Address string `json:"address"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Address == "" {
		jsonError(w, "address required", http.StatusBadRequest)
		return
	}
	nonce := s.authSvc.IssueNonce(req.Address)
	jsonOK(w, map[string]string{"nonce": nonce})
}

// handleAuthVerify verifies a SIWE-style message+signature.
// For this implementation we accept the wallet address + nonce + raw signature.
// A production build should parse the full EIP-4361 message.
func (s *Server) handleAuthVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Address   string `json:"address"`
		Nonce     string `json:"nonce"`
		Signature string `json:"signature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	if req.Address == "" || req.Nonce == "" || req.Signature == "" {
		jsonError(w, "address, nonce, and signature required", http.StatusBadRequest)
		return
	}
	if err := s.authSvc.ConsumeNonce(req.Address, req.Nonce); err != nil {
		jsonError(w, fmt.Sprintf("nonce invalid: %v", err), http.StatusUnauthorized)
		return
	}

	ctx := r.Context()
	team, err := s.db.GetOrCreateTeamByWallet(ctx, req.Address)
	if err != nil {
		log.Printf("[api] GetOrCreateTeamByWallet: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	token, err := s.authSvc.IssueJWT(req.Address, team.ID)
	if err != nil {
		jsonError(w, "could not issue token", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"token": token, "team_id": team.ID})
}

// --- Sessions ---

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	teamID := r.Context().Value(ctxKeyTeamID).(string)
	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Prompt == "" {
		jsonError(w, "prompt required", http.StatusBadRequest)
		return
	}

	sessionID := newUUID()
	ctx := r.Context()
	if err := s.db.CreateSession(ctx, &store.Session{
		ID:     sessionID,
		TeamID: teamID,
		Prompt: req.Prompt,
		State:  "running",
	}); err != nil {
		log.Printf("[api] CreateSession: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	sess := agent.NewSession(
		sessionID, teamID,
		s.mgr, s.sc,
		s.cfg.AnthropicAPIKey, s.cfg.AgentModel,
		s.cfg.EthSepolia_RPC_URL, s.cfg.ProviderRegistryAddress,
		s.cfg.DeployDomain,
		s.zeroG,
	)

	s.mu.Lock()
	s.sessions[sessionID] = sess
	s.mu.Unlock()

	// Run agent in background
	go func() {
		runCtx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
		defer cancel()
		if err := sess.Run(runCtx, req.Prompt); err != nil {
			log.Printf("[api] session %s failed: %v", sessionID, err)
		}

		// Persist action log and update session state in DB
		if err := s.db.UpsertActionLog(runCtx, sessionID, teamID, sess.Actions); err != nil {
			log.Printf("[api] UpsertActionLog: %v", err)
		}
		if err := s.db.UpdateSessionState(runCtx, sessionID, string(sess.State)); err != nil {
			log.Printf("[api] UpdateSessionState: %v", err)
		}
		if err := s.db.UpdateSessionMerkleRoot(runCtx, sessionID, sess.MerkleRoot, ""); err != nil {
			log.Printf("[api] UpdateSessionMerkleRoot: %v", err)
		}

		// Submit EAS attestation
		if s.cfg.AgentWalletPrivateKey != "" && s.cfg.EASSchemaUID != "" && sess.MerkleRoot != "" {
			merkleArr, _ := chain.HexToBytes32(sess.MerkleRoot)
			result, err := chain.SubmitAttestation(runCtx,
				s.cfg.EthSepolia_RPC_URL, s.cfg.AgentWalletPrivateKey,
				s.cfg.EASSchemaUID, sessionID, teamID, merkleArr)
			if err != nil {
				log.Printf("[api] SubmitAttestation: %v", err)
			} else {
				// Wait for the attestation UID from the mined receipt
				uid, uidErr := chain.WaitForAttestationUID(runCtx, s.cfg.EthSepolia_RPC_URL, result.TxHash)
				if uidErr != nil {
					log.Printf("[api] WaitForAttestationUID: %v", uidErr)
				}
				easScanURL := ""
				if uid != "" {
					easScanURL = "https://sepolia.easscan.org/attestation/view/" + uid
				}
				_ = s.db.CreateAttestation(runCtx, &store.Attestation{
					SessionID:      sessionID,
					TxHash:         result.TxHash,
					AttestationUID: uid,
					MerkleRoot:     sess.MerkleRoot,
					SchemaUID:      s.cfg.EASSchemaUID,
					EASScanURL:     easScanURL,
				})
				_ = s.db.UpdateSessionMerkleRoot(runCtx, sessionID, sess.MerkleRoot, result.TxHash)
				// Register keeperhub follow-up job
				_, _ = s.keeper.RegisterJob(runCtx, keeperhub.Job{
					Name:      "confirm-attestation",
					SessionID: sessionID,
					Payload: map[string]any{
						"tx_hash":         result.TxHash,
						"attestation_uid": uid,
						"merkle_root":     sess.MerkleRoot,
					},
				})
			}
		}
	}()

	jsonOK(w, map[string]string{"session_id": sessionID})
}

func (s *Server) handleGetSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sess := s.lookupSession(id)
	if sess == nil {
		jsonError(w, "session not found", http.StatusNotFound)
		return
	}
	jsonOK(w, map[string]any{
		"id":          sess.ID,
		"team_id":     sess.TeamID,
		"state":       sess.State,
		"action_count": len(sess.Actions),
		"merkle_root": sess.MerkleRoot,
	})
}

func (s *Server) handleConfirmSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sess := s.lookupSession(id)
	if sess == nil {
		jsonError(w, "session not found", http.StatusNotFound)
		return
	}
	sess.Confirm()
	jsonOK(w, map[string]string{"status": "confirmed"})
}

func (s *Server) handleGetAudit(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var actions []agent.Action
	var merkleRoot string

	// Try in-memory session first (active or recently completed)
	if sess := s.lookupSession(id); sess != nil {
		actions = sess.Actions
		merkleRoot = sess.MerkleRoot
	} else {
		// Fall back to persisted action log in DB
		al, err := s.db.GetActionLog(r.Context(), id)
		if err != nil {
			jsonError(w, "session not found", http.StatusNotFound)
			return
		}
		if err := json.Unmarshal(al.Actions, &actions); err != nil {
			jsonError(w, "failed to decode action log", http.StatusInternalServerError)
			return
		}
		if dbSess, err := s.db.GetSession(r.Context(), id); err == nil {
			merkleRoot = dbSess.MerkleRoot
		}
	}

	type auditEntry struct {
		agent.Action
		Proof []string `json:"proof"`
	}
	entries := make([]auditEntry, len(actions))
	for i, a := range actions {
		entries[i] = auditEntry{Action: a, Proof: agent.ComputeMerkleProof(actions, i)}
	}

	jsonOK(w, map[string]any{
		"session_id":  id,
		"merkle_root": merkleRoot,
		"actions":     entries,
	})
}

func (s *Server) handleStreamSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sess := s.lookupSession(id)
	if sess == nil {
		jsonError(w, "session not found", http.StatusNotFound)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[api] ws upgrade: %v", err)
		return
	}
	defer conn.Close()

	events := sess.Events()
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}
			b, _ := json.Marshal(event)
			if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
				return
			}
			if event.Type == "done" || event.Type == "error" {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}

// --- Providers ---

func (s *Server) handleGetProviders(w http.ResponseWriter, r *http.Request) {
	providers, err := chain.GetActiveProviders(r.Context(), s.cfg.EthSepolia_RPC_URL, s.cfg.ProviderRegistryAddress)
	if err != nil {
		log.Printf("[api] GetActiveProviders: %v", err)
		jsonError(w, "could not fetch providers", http.StatusInternalServerError)
		return
	}
	type providerView struct {
		Wallet        string `json:"wallet"`
		Endpoint      string `json:"endpoint"`
		PricePerHour  string `json:"price_per_hour"`
		JobsCompleted uint64 `json:"jobs_completed"`
	}
	views := make([]providerView, len(providers))
	for i, p := range providers {
		views[i] = providerView{
			Wallet:        p.Wallet.Hex(),
			Endpoint:      p.Endpoint,
			PricePerHour:  p.PricePerHour.String(),
			JobsCompleted: p.JobsCompleted.Uint64(),
		}
	}
	jsonOK(w, views)
}

// --- Workspaces ---

func (s *Server) handleListWorkspaces(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "id")
	containers, err := s.db.ListTeamContainers(r.Context(), teamID)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, containers)
}

// --- Vault ---

func (s *Server) handleVaultNonce(w http.ResponseWriter, r *http.Request) {
	address := r.Context().Value(ctxKeyAddress).(string)
	nonce := s.authSvc.IssueNonce("vault:" + address)
	jsonOK(w, map[string]string{"nonce": nonce})
}

func (s *Server) handleVaultKey(w http.ResponseWriter, r *http.Request) {
	address := r.Context().Value(ctxKeyAddress).(string)
	var req struct {
		ContainerID string `json:"container_id"`
		Nonce       string `json:"nonce"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ContainerID == "" {
		jsonError(w, "container_id required", http.StatusBadRequest)
		return
	}
	if err := s.authSvc.ConsumeNonce("vault:"+address, req.Nonce); err != nil {
		jsonError(w, "invalid nonce", http.StatusUnauthorized)
		return
	}
	vaultKey := chain.DeriveVaultKey(s.cfg.VaultMasterSecret, req.ContainerID)
	jsonOK(w, map[string]string{"key": vaultKey})
}

// --- x402 middleware ---

// x402Middleware checks for a valid USDC transferWithAuthorization header.
// On missing or invalid payment it returns 402 with details.
func (s *Server) x402Middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		paymentHeader := r.Header.Get("X-Payment")
		if paymentHeader == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusPaymentRequired)
			json.NewEncoder(w).Encode(map[string]any{
				"error":             "payment_required",
				"payment_required":  true,
				"usdc_contract":     chain.USDCAddress,
			"recipient":         chain.AgentAddress(s.cfg.AgentWalletPrivateKey),
				"amount":            "1000000",                    // 1 USDC (6 decimals)
				"chain_id":          11155111,
				"payment_type":      "eip3009",
			})
			return
		}

		// Decode and verify the payment proof
		raw, err := base64.StdEncoding.DecodeString(paymentHeader)
		if err != nil {
			jsonError(w, "invalid X-Payment header encoding", http.StatusBadRequest)
			return
		}
		var auth chain.TransferAuth
		if err := json.Unmarshal(raw, &auth); err != nil {
			jsonError(w, "invalid X-Payment payload", http.StatusBadRequest)
			return
		}
		if err := chain.VerifyTransferAuth(auth); err != nil {
			jsonError(w, fmt.Sprintf("payment verification failed: %v", err), http.StatusPaymentRequired)
			return
		}

		// Idempotency — prevent replay
		ctx := r.Context()
		if exists, _ := s.db.PaymentNonceExists(ctx, fmt.Sprintf("%x", auth.Nonce)); exists {
			jsonError(w, "payment already used", http.StatusPaymentRequired)
			return
		}
		if err := s.db.CreatePayment(ctx, &store.Payment{
			Nonce:      fmt.Sprintf("%x", auth.Nonce),
			Wallet:     auth.From.Hex(),
			AmountUSDC: auth.Value.String(),
			Status:     "pending",
		}); err != nil {
			log.Printf("[api] RecordPayment: %v", err)
		}

		// Execute on-chain in background (fire-and-forget for demo; real impl should wait)
		go func() {
			txHash, err := chain.ExecuteTransferAuth(
				context.Background(), s.cfg.EthSepolia_RPC_URL,
				s.cfg.AgentWalletPrivateKey, auth)
			if err != nil {
				log.Printf("[api] ExecuteTransferAuth: %v", err)
			} else {
				log.Printf("[api] USDC transfer tx: %s", txHash)
			}
		}()

		next.ServeHTTP(w, r)
	}
}

// --- JWT middleware ---

type contextKey string

const (
	ctxKeyAddress contextKey = "address"
	ctxKeyTeamID  contextKey = "team_id"
)

func (s *Server) jwtMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			jsonError(w, "authorization required", http.StatusUnauthorized)
			return
		}
		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		address, teamID, err := s.authSvc.ValidateJWT(tokenStr)
		if err != nil {
			jsonError(w, "invalid token", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeyAddress, address)
		ctx = context.WithValue(ctx, ctxKeyTeamID, teamID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// --- CORS middleware ---

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Payment")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- Account ---

// handleGetAccount returns the team record for a wallet address (no JWT required).
// Used by AuthContext.fetchAccount on every page load.
func (s *Server) handleGetAccount(w http.ResponseWriter, r *http.Request) {
	wallet := r.URL.Query().Get("wallet")
	if wallet == "" {
		jsonError(w, "wallet query param required", http.StatusBadRequest)
		return
	}
	team, err := s.db.GetOrCreateTeamByWallet(r.Context(), strings.ToLower(wallet))
	if err != nil {
		log.Printf("[api] GetOrCreateTeamByWallet: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{
		"id":     team.ID,
		"name":   team.Name,
		"wallet": team.Wallet,
	})
}

// handleUpdateAccount updates the team display name (used by the onboarding flow).
func (s *Server) handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
	teamID := r.Context().Value(ctxKeyTeamID).(string)
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		jsonError(w, "name required", http.StatusBadRequest)
		return
	}
	if err := s.db.UpdateTeamName(r.Context(), teamID, req.Name); err != nil {
		log.Printf("[api] UpdateTeamName: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"team_id": teamID, "team_name": req.Name})
}

// --- Team resources ---

func (s *Server) handleListTeamSessions(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "id")
	sessions, err := s.db.ListTeamSessions(r.Context(), teamID)
	if err != nil {
		log.Printf("[api] ListTeamSessions: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if sessions == nil {
		sessions = []store.Session{}
	}
	jsonOK(w, sessions)
}

func (s *Server) handleListTeamAttestations(w http.ResponseWriter, r *http.Request) {
	teamID := chi.URLParam(r, "id")
	attestations, err := s.db.ListTeamAttestations(r.Context(), teamID)
	if err != nil {
		log.Printf("[api] ListTeamAttestations: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if attestations == nil {
		attestations = []store.Attestation{}
	}
	jsonOK(w, attestations)
}

// --- Payments ---

func (s *Server) handleListPayments(w http.ResponseWriter, r *http.Request) {
	address := r.Context().Value(ctxKeyAddress).(string)
	payments, err := s.db.ListPaymentsByWallet(r.Context(), address)
	if err != nil {
		log.Printf("[api] ListPaymentsByWallet: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if payments == nil {
		payments = []store.Payment{}
	}
	jsonOK(w, payments)
}

// --- Secrets ---

func (s *Server) handleListSecrets(w http.ResponseWriter, r *http.Request) {
	teamID := r.Context().Value(ctxKeyTeamID).(string)
	secrets, err := s.db.ListSecrets(r.Context(), teamID)
	if err != nil {
		log.Printf("[api] ListSecrets: %v", err)
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if secrets == nil {
		secrets = []store.Secret{}
	}
	jsonOK(w, secrets)
}

func (s *Server) handleCreateSecret(w http.ResponseWriter, r *http.Request) {
	teamID := r.Context().Value(ctxKeyTeamID).(string)
	var req struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.Value == "" {
		jsonError(w, "name and value required", http.StatusBadRequest)
		return
	}
	// Normalise: uppercase + underscores
	name := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(req.Name), " ", "_"))
	sec, err := s.db.CreateSecret(r.Context(), teamID, name, req.Value)
	if err != nil {
		log.Printf("[api] CreateSecret: %v", err)
		jsonError(w, "could not create secret (name may already exist)", http.StatusBadRequest)
		return
	}
	jsonOK(w, sec)
}

func (s *Server) handleDeleteSecret(w http.ResponseWriter, r *http.Request) {
	teamID := r.Context().Value(ctxKeyTeamID).(string)
	idStr := chi.URLParam(r, "id")
	var id int64
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil || id <= 0 {
		jsonError(w, "invalid secret id", http.StatusBadRequest)
		return
	}
	if err := s.db.DeleteSecret(r.Context(), id, teamID); err != nil {
		log.Printf("[api] DeleteSecret: %v", err)
		jsonError(w, "could not delete secret", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- helpers ---

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (s *Server) lookupSession(id string) *agent.Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[id]
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
