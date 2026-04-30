package agent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/comput3ai/comput3/backend/internal/chain"
	"github.com/comput3ai/comput3/backend/internal/container"
	"github.com/comput3ai/comput3/backend/internal/scanner"
)

// ZeroGClient is the interface for 0G Network integration.
// The agent calls these methods after each action to persist state.
type ZeroGClient interface {
	Append(ctx context.Context, logID string, entry []byte) error
	ReadLog(ctx context.Context, logID string) ([][]byte, error)
}

// Action represents a single tool call in the audit log.
type Action struct {
	Index     int            `json:"index"`
	Tool      string         `json:"tool"`
	Input     map[string]any `json:"input"`
	Result    any            `json:"result"`
	Error     string         `json:"error,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Hash      string         `json:"hash"` // SHA256(index|tool|input|result|timestamp)
}

// SessionState is the lifecycle state of an agent session.
type SessionState string

const (
	StateRunning   SessionState = "running"
	StateCompleted SessionState = "completed"
	StateFailed    SessionState = "failed"
)

// Event is streamed to the frontend over WebSocket.
type Event struct {
	Type        string  `json:"type"` // action | message | plan | done | error
	Action      *Action `json:"action,omitempty"`
	Message     string  `json:"message,omitempty"`
	Plan        any     `json:"plan,omitempty"`
	ContainerID string  `json:"container_id,omitempty"`
	DeployedURL string  `json:"deployed_url,omitempty"`
	MerkleRoot  string  `json:"merkle_root,omitempty"`
}

// Session manages one agent deployment conversation.
type Session struct {
	ID               string
	TeamID           string
	State            SessionState
	Actions          []Action
	MerkleRoot       string
	Plan             *scanner.DeploymentPlan
	SelectedProvider *chain.Provider

	mgr             *container.Manager
	scanner         *scanner.Scanner
	anthropicAPIKey string
	model           string
	rpcURL          string
	registryAddress string
	auctionAddress  string  // JobAuction contract — empty means skip auction
	agentPrivKey    string  // agent wallet key, used to postJob and closeAuction
	deployDomain    string
	zeroG           ZeroGClient

	events          chan Event
	confirmCh       chan struct{}
	lastContainerID string
}

// anthropic API types
type anthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []contentBlock
}

type contentBlock struct {
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
	Content   string         `json:"content,omitempty"`
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system"`
	Tools     []map[string]any   `json:"tools"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicResponse struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Content    []contentBlock `json:"content"`
	Model      string         `json:"model"`
	StopReason string         `json:"stop_reason"`
	Error      *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// NewSession creates a new agent session.
func NewSession(
	id, teamID string,
	mgr *container.Manager,
	sc *scanner.Scanner,
	anthropicAPIKey, model, rpcURL, registryAddress, auctionAddress, agentPrivKey, deployDomain string,
	zeroG ZeroGClient,
) *Session {
	if model == "" {
		model = "claude-opus-4-5"
	}
	return &Session{
		ID:              id,
		TeamID:          teamID,
		State:           StateRunning,
		mgr:             mgr,
		scanner:         sc,
		anthropicAPIKey: anthropicAPIKey,
		model:           model,
		rpcURL:          rpcURL,
		registryAddress: registryAddress,
		auctionAddress:  auctionAddress,
		agentPrivKey:    agentPrivKey,
		deployDomain:    deployDomain,
		zeroG:           zeroG,
		events:          make(chan Event, 64),
		confirmCh:       make(chan struct{}),
	}
}

// Confirm unblocks the agent if it is waiting for plan confirmation.
func (s *Session) Confirm() {
	select {
	case <-s.confirmCh:
	default:
		close(s.confirmCh)
	}
}

// Events returns the read-only event channel.
func (s *Session) Events() <-chan Event {
	return s.events
}

// Run executes the agent loop for the given user prompt.
func (s *Session) Run(ctx context.Context, userPrompt string) error {
	messages := []anthropicMessage{
		{Role: "user", Content: userPrompt},
	}
	consecutiveNoToolTurns := 0

	for {
		log.Printf("[session %s] calling Claude (turn %d)...", s.ID, len(messages))
		resp, err := s.callClaude(ctx, messages)
		if err != nil {
			log.Printf("[session %s] Claude error: %v", s.ID, err)
			s.State = StateFailed
			s.emit(Event{Type: "error", Message: err.Error()})
			return err
		}
		log.Printf("[session %s] Claude responded: stop_reason=%s blocks=%d", s.ID, resp.StopReason, len(resp.Content))

		var assistantBlocks []contentBlock
		for _, block := range resp.Content {
			assistantBlocks = append(assistantBlocks, block)
			if block.Type == "text" && block.Text != "" {
				s.emit(Event{Type: "message", Message: block.Text})
			}
		}

		hasToolUse := false
		for _, block := range resp.Content {
			if block.Type == "tool_use" {
				hasToolUse = true
				break
			}
		}

		if !hasToolUse {
			consecutiveNoToolTurns++
			if consecutiveNoToolTurns <= 2 {
				s.emit(Event{Type: "message", Message: "Model returned text without tool calls; requesting tool-only response..."})
				messages = append(messages, anthropicMessage{Role: "user",
					Content: "Use at least one tool call now. If a GitHub URL exists, call analyze_repo first."})
				continue
			}
			if len(s.Actions) == 0 {
				if repoURL := extractGitHubURL(userPrompt); repoURL != "" {
					s.emit(Event{Type: "message", Message: "Bootstrapping deployment planning..."})
					if err := s.bootstrapInitialPlan(ctx, repoURL); err != nil {
						s.State = StateFailed
						s.emit(Event{Type: "error", Message: err.Error()})
						return err
					}
					consecutiveNoToolTurns = 0
					messages = append(messages, anthropicMessage{Role: "user",
						Content: "Planning is confirmed. Continue deployment by calling tools only."})
					continue
				}
			}
			break
		}
		consecutiveNoToolTurns = 0

		var toolResults []contentBlock
		for _, block := range resp.Content {
			if block.Type != "tool_use" {
				continue
			}
			log.Printf("[session %s] executing tool: %s", s.ID, block.Name)
			result, toolErr := s.executeTool(ctx, block.Name, block.Input)
			log.Printf("[session %s] tool %s done (err=%v)", s.ID, block.Name, toolErr)

			action := Action{
				Index:     len(s.Actions),
				Tool:      block.Name,
				Input:     block.Input,
				Timestamp: time.Now().UTC(),
			}
			if toolErr != nil {
				action.Error = toolErr.Error()
			} else {
				action.Result = result
			}
			action.Hash = hashAction(action)
			s.Actions = append(s.Actions, action)
			s.emit(Event{Type: "action", Action: &action})

			// Persist to 0G Network after each action
			if s.zeroG != nil {
				if b, err := json.Marshal(action); err == nil {
					if err := s.zeroG.Append(ctx, s.ID, b); err != nil {
						log.Printf("[session %s] 0G append: %v", s.ID, err)
					}
				}
			}

			var resultContent string
			if toolErr != nil {
				resultContent = fmt.Sprintf("error: %s", toolErr.Error())
			} else {
				b, _ := json.Marshal(result)
				resultContent = string(b)
			}
			toolResults = append(toolResults, contentBlock{
				Type:      "tool_result",
				ToolUseID: block.ID,
				Content:   resultContent,
			})
		}

		messages = append(messages,
			anthropicMessage{Role: "assistant", Content: assistantBlocks},
			anthropicMessage{Role: "user", Content: toolResults},
		)
	}

	// Compute Merkle root over all action hashes
	s.MerkleRoot = computeMerkleRoot(s.Actions)
	s.State = StateCompleted

	doneEvent := Event{Type: "done", Message: "Deployment complete.", MerkleRoot: s.MerkleRoot}
	if s.lastContainerID != "" {
		doneEvent.ContainerID = s.lastContainerID
		if s.deployDomain != "" {
			doneEvent.DeployedURL = fmt.Sprintf("https://%s.%s", s.lastContainerID, s.deployDomain)
		}
	}
	s.emit(doneEvent)
	return nil
}

// executeTool dispatches named tool calls.
func (s *Session) executeTool(ctx context.Context, name string, input map[string]any) (any, error) {
	switch name {
	case "select_provider":
		s.emit(Event{Type: "message", Message: "Selecting compute provider..."})
		if s.auctionAddress != "" && s.agentPrivKey != "" {
			return s.selectProviderViaAuction(ctx)
		}
		return s.selectProviderFromRegistry(ctx)

	case "analyze_repo":
		url := sanitizeGitHubURL(stringField(input, "github_url"))
		if url == "" {
			return nil, fmt.Errorf("github_url is required")
		}
		s.emit(Event{Type: "message", Message: fmt.Sprintf("Scanning repository: %s", url)})
		plan, err := s.scanner.AnalyzeRepo(ctx, url)
		if err != nil {
			return nil, err
		}
		s.Plan = plan
		return plan, nil

	case "generate_deployment_plan":
		plan := map[string]any{
			"summary":                stringField(input, "summary"),
			"estimated_cost_per_hour": float64Field(input, "estimated_cost_per_hour", 0),
			"containers":             input["containers"],
			"has_smart_contracts":    input["has_smart_contracts"],
			"status":                 "awaiting_confirmation",
		}
		s.emit(Event{Type: "plan", Plan: plan})

		timer := time.NewTimer(10 * time.Minute)
		defer timer.Stop()
		select {
		case <-s.confirmCh:
			plan["status"] = "confirmed"
			s.emit(Event{Type: "message", Message: "Plan confirmed — starting deployment..."})
		case <-timer.C:
			return nil, fmt.Errorf("deployment plan timed out waiting for confirmation")
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return plan, nil

	case "create_container":
		opts := container.CreateOpts{
			TeamID:    s.TeamID,
			SessionID: s.ID,
			Name:      stringField(input, "name"),
			Image:     stringField(input, "image"),
			RAMMb:     int64Field(input, "ram_mb", 2048),
			CPUCores:  float64Field(input, "cpu_cores", 1.0),
		}
		if ports, ok := input["ports"].([]any); ok {
			for _, p := range ports {
				if ps, ok := p.(string); ok {
					opts.Ports = append(opts.Ports, ps)
				}
			}
		}
		info, err := s.mgr.CreateContainer(ctx, opts)
		if err == nil && info != nil {
			s.mgr.RegisterDeploy(info.ID, info.Ports)
			s.lastContainerID = info.ID
		}
		return info, err

	case "install_packages":
		id := stringField(input, "container_id")
		mgr := container.PackageManager(stringField(input, "manager"))
		var pkgs []string
		if raw, ok := input["packages"].([]any); ok {
			for _, p := range raw {
				if ps, ok := p.(string); ok {
					pkgs = append(pkgs, ps)
				}
			}
		}
		return nil, s.mgr.InstallPackages(ctx, id, pkgs, mgr)

	case "configure_network":
		var ids []string
		if raw, ok := input["container_ids"].([]any); ok {
			for _, p := range raw {
				if ps, ok := p.(string); ok {
					ids = append(ids, ps)
				}
			}
		}
		if err := s.mgr.CreateNetwork(ctx, s.TeamID); err != nil {
			return nil, err
		}
		return nil, s.mgr.ConnectContainers(ctx, s.TeamID, ids)

	case "setup_ide":
		return nil, s.mgr.SetupIDE(ctx, stringField(input, "container_id"),
			container.IDEType(stringField(input, "type")))

	case "setup_database":
		return s.mgr.SetupDatabase(ctx, s.TeamID, s.ID,
			container.DBType(stringField(input, "type")),
			stringField(input, "version"))

	case "health_check":
		return s.mgr.HealthCheck(ctx, stringField(input, "container_id"))

	case "get_logs":
		logs, err := s.mgr.GetLogs(ctx, stringField(input, "container_id"),
			int(int64Field(input, "lines", 50)))
		return map[string]string{"logs": logs}, err

	case "destroy_container":
		return nil, s.mgr.Destroy(ctx, stringField(input, "container_id"))

	case "clone_repo":
		id := stringField(input, "container_id")
		url := sanitizeGitHubURL(stringField(input, "github_url"))
		if url == "" {
			return nil, fmt.Errorf("github_url is required")
		}
		s.emit(Event{Type: "message", Message: fmt.Sprintf("Cloning %s into %s...", url, id)})
		out, err := s.mgr.CloneRepo(ctx, id, url, stringField(input, "directory"))
		return map[string]string{"output": out}, err

	case "run_command":
		id := stringField(input, "container_id")
		workDir := stringField(input, "work_dir")
		if workDir == "" {
			workDir = "/app"
		}
		s.emit(Event{Type: "message", Message: fmt.Sprintf("Running: %s", stringField(input, "command"))})
		out, err := s.mgr.RunCommand(ctx, id, stringField(input, "command"), workDir, mapField(input, "env"))
		return map[string]string{"output": out}, err

	case "start_process":
		id := stringField(input, "container_id")
		workDir := stringField(input, "work_dir")
		if workDir == "" {
			workDir = "/app"
		}
		s.emit(Event{Type: "message", Message: fmt.Sprintf("Starting process: %s", stringField(input, "command"))})
		out, err := s.mgr.StartProcess(ctx, id, stringField(input, "command"), workDir, mapField(input, "env"))
		return map[string]string{"output": out}, err

	case "write_file":
		path := stringField(input, "path")
		s.emit(Event{Type: "message", Message: fmt.Sprintf("Writing %s", path)})
		return map[string]string{"path": path},
			s.mgr.WriteFile(ctx, stringField(input, "container_id"), path, stringField(input, "content"))

	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

// callClaude sends the conversation to the Anthropic Messages API.
func (s *Session) callClaude(ctx context.Context, messages []anthropicMessage) (*anthropicResponse, error) {
	req := anthropicRequest{
		Model:     s.model,
		MaxTokens: 8192,
		System:    systemPrompt,
		Tools:     toolDefinitions,
		Messages:  messages,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", s.anthropicAPIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic request: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, err
	}
	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic error %d: %s", httpResp.StatusCode, string(respBody))
	}

	var resp anthropicResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parse anthropic response: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("anthropic error: %s", resp.Error.Message)
	}
	return &resp, nil
}

// selectProviderFromRegistry reads the ProviderRegistry directly, picks the cheapest
// active provider, and health-checks its endpoint before returning.
func (s *Session) selectProviderFromRegistry(ctx context.Context) (any, error) {
	s.emit(Event{Type: "message", Message: "Querying ProviderRegistry on Ethereum Sepolia..."})
	provider, err := chain.SelectCheapestProvider(ctx, s.rpcURL, s.registryAddress)
	if err != nil {
		s.emit(Event{Type: "message", Message: fmt.Sprintf("Warning: chain query failed (%v). Using local node.", err)})
		return map[string]any{"endpoint": "http://localhost:8081", "price_per_hour": "0", "source": "fallback"}, nil
	}
	// Health-check the provider before accepting.
	healthURL := strings.TrimRight(provider.Endpoint, "/") + "/health"
	pingCtx, pingCancel := context.WithTimeout(ctx, 3*time.Second)
	defer pingCancel()
	if req, perr := http.NewRequestWithContext(pingCtx, http.MethodGet, healthURL, nil); perr == nil {
		if resp, perr := http.DefaultClient.Do(req); perr != nil || resp.StatusCode >= 500 {
			s.emit(Event{Type: "message", Message: fmt.Sprintf("Warning: provider health check failed (%v). Using local node.", perr)})
			return map[string]any{"endpoint": "http://localhost:8081", "price_per_hour": "0", "source": "fallback"}, nil
		}
	}
	s.SelectedProvider = provider
	return map[string]any{
		"wallet":         provider.Wallet.Hex(),
		"endpoint":       provider.Endpoint,
		"price_per_hour": provider.PricePerHour.String(),
		"jobs_completed": provider.JobsCompleted.Uint64(),
		"source":         "on-chain",
	}, nil
}

// selectProviderViaAuction posts a job to the JobAuction contract, waits 30 s for
// provider bids, closes the auction, and returns the winning provider.
// Falls back to selectProviderFromRegistry on any error.
func (s *Session) selectProviderViaAuction(ctx context.Context) (any, error) {
	jobID := chain.SessionIDToJobID(s.ID)

	// Derive resource requirements from the deployment plan.
	ramMb := big.NewInt(2048)
	cpuCores := big.NewInt(1)
	if s.Plan != nil && len(s.Plan.Containers) > 0 {
		var totalRAM int64
		for _, c := range s.Plan.Containers {
			totalRAM += c.RAMMb
			if cores := int64(c.CPUCores); cores > cpuCores.Int64() {
				cpuCores = big.NewInt(cores)
			}
		}
		if totalRAM > 0 {
			ramMb = big.NewInt(totalRAM)
		}
		if cpuCores.Sign() == 0 {
			cpuCores = big.NewInt(1)
		}
	}

	// Ceiling price: 0.01 ETH/hr. Deposit: same (covers exactly 1 hr).
	maxPricePerHour := big.NewInt(10_000_000_000_000_000) // 0.01 ETH/hr
	durationSeconds := big.NewInt(3600)
	depositWei := big.NewInt(10_000_000_000_000_000) // 0.01 ETH

	s.emit(Event{Type: "message", Message: "Posting job to on-chain auction (30 s bid window)..."})
	_, err := chain.PostJob(ctx, s.rpcURL, s.agentPrivKey, s.auctionAddress,
		jobID, maxPricePerHour, ramMb, cpuCores, durationSeconds, depositWei)
	if err != nil {
		s.emit(Event{Type: "message", Message: fmt.Sprintf("Auction post failed (%v) — falling back to registry.", err)})
		return s.selectProviderFromRegistry(ctx)
	}

	s.emit(Event{Type: "message", Message: "Job posted on-chain — waiting 30 s for provider bids..."})

	// Wait for bid window, then close the auction.
	watchCtx, watchCancel := context.WithTimeout(ctx, 55*time.Second)
	defer watchCancel()

	// Fire closeAuction ~1 s after the 30 s window expires.
	go func() {
		select {
		case <-time.After(31 * time.Second):
			if _, cerr := chain.CloseAuction(watchCtx, s.rpcURL, s.agentPrivKey, s.auctionAddress, jobID); cerr != nil {
				log.Printf("[session %s] CloseAuction: %v", s.ID, cerr)
			} else {
				log.Printf("[session %s] CloseAuction submitted", s.ID)
			}
		case <-watchCtx.Done():
		}
	}()

	s.emit(Event{Type: "message", Message: "Waiting for auction result..."})
	awarded, err := chain.WatchJobAwarded(watchCtx, s.rpcURL, s.auctionAddress, jobID)
	if err != nil {
		s.emit(Event{Type: "message", Message: fmt.Sprintf("No auction result (%v) — falling back to registry.", err)})
		return s.selectProviderFromRegistry(ctx)
	}

	source := "auction-winner"
	if awarded.IsFallback {
		source = "auction-fallback"
	}
	s.emit(Event{Type: "message", Message: fmt.Sprintf("Auction closed — winner: %s at %s wei/hr (%s)", awarded.Winner.Hex(), awarded.PricePerHour, source)})

	// Look up the winner's endpoint from the registry.
	providers, err := chain.GetActiveProviders(ctx, s.rpcURL, s.registryAddress)
	if err == nil {
		for i := range providers {
			if providers[i].Wallet == awarded.Winner {
				s.SelectedProvider = &providers[i]
				return map[string]any{
					"wallet":          providers[i].Wallet.Hex(),
					"endpoint":        providers[i].Endpoint,
					"price_per_hour":  awarded.PricePerHour.String(),
					"rate_per_second": awarded.RatePerSecond.String(),
					"jobs_completed":  providers[i].JobsCompleted.Uint64(),
					"source":          source,
				}, nil
			}
		}
	}

	// Winner not in current registry view — construct minimal provider from event data.
	s.SelectedProvider = &chain.Provider{
		Wallet:       awarded.Winner,
		PricePerHour: awarded.PricePerHour,
		Active:       true,
		Endpoint:     "http://localhost:8081",
	}
	return map[string]any{
		"wallet":          awarded.Winner.Hex(),
		"price_per_hour":  awarded.PricePerHour.String(),
		"rate_per_second": awarded.RatePerSecond.String(),
		"source":          source,
	}, nil
}

func (s *Session) bootstrapInitialPlan(ctx context.Context, repoURL string) error {
	analyzeResult, err := s.executeToolAndRecord(ctx, "analyze_repo", map[string]any{"github_url": repoURL})
	if err != nil {
		return err
	}
	_, _ = s.executeToolAndRecord(ctx, "select_provider", map[string]any{})

	plan, ok := analyzeResult.(*scanner.DeploymentPlan)
	if !ok || plan == nil {
		return fmt.Errorf("bootstrap analyze_repo returned invalid plan")
	}
	_, err = s.executeToolAndRecord(ctx, "generate_deployment_plan", map[string]any{
		"summary":                 plan.Summary,
		"estimated_cost_per_hour": plan.EstimatedCostPerHour,
		"containers":              plan.Containers,
		"has_smart_contracts":     plan.HasSmartContracts,
	})
	return err
}

func (s *Session) executeToolAndRecord(ctx context.Context, name string, input map[string]any) (any, error) {
	result, toolErr := s.executeTool(ctx, name, input)
	action := Action{
		Index:     len(s.Actions),
		Tool:      name,
		Input:     input,
		Timestamp: time.Now().UTC(),
	}
	if toolErr != nil {
		action.Error = toolErr.Error()
	} else {
		action.Result = result
	}
	action.Hash = hashAction(action)
	s.Actions = append(s.Actions, action)
	s.emit(Event{Type: "action", Action: &action})
	if s.zeroG != nil {
		if b, err := json.Marshal(action); err == nil {
			if err := s.zeroG.Append(ctx, s.ID, b); err != nil {
				log.Printf("[session %s] 0G append: %v", s.ID, err)
			}
		}
	}
	return result, toolErr
}

func (s *Session) emit(e Event) {
	select {
	case s.events <- e:
	default:
	}
}

// --- Merkle root computation ---

// hashAction computes SHA256(index|tool|JSON(input)|JSON(result)|ISO8601(timestamp)).
func hashAction(a Action) string {
	resultBytes, _ := json.Marshal(a.Result)
	inputBytes, _ := json.Marshal(a.Input)
	preimage := fmt.Sprintf("%d|%s|%s|%s|%s",
		a.Index, a.Tool, string(inputBytes), string(resultBytes), a.Timestamp.UTC().Format(time.RFC3339))
	sum := sha256.Sum256([]byte(preimage))
	return fmt.Sprintf("%x", sum)
}

// computeMerkleRoot builds a binary Merkle tree over action hashes and returns the root.
func computeMerkleRoot(actions []Action) string {
	if len(actions) == 0 {
		return fmt.Sprintf("%x", sha256.Sum256([]byte{}))
	}
	leaves := make([][32]byte, len(actions))
	for i, a := range actions {
		h, _ := hexToHash(a.Hash)
		leaves[i] = h
	}
	root := merkleRoot(leaves)
	return fmt.Sprintf("%x", root)
}

func merkleRoot(nodes [][32]byte) [32]byte {
	if len(nodes) == 1 {
		return nodes[0]
	}
	if len(nodes)%2 != 0 {
		nodes = append(nodes, nodes[len(nodes)-1]) // duplicate last leaf
	}
	var next [][32]byte
	for i := 0; i < len(nodes); i += 2 {
		combined := append(nodes[i][:], nodes[i+1][:]...)
		next = append(next, sha256.Sum256(combined))
	}
	return merkleRoot(next)
}

// ComputeMerkleProof returns the sibling hashes from leaf to root for the action at leafIndex.
// Each entry is prefixed "left:<hex>" or "right:<hex>" indicating which side the sibling sits on.
func ComputeMerkleProof(actions []Action, leafIndex int) []string {
	if len(actions) == 0 || leafIndex < 0 || leafIndex >= len(actions) {
		return nil
	}
	leaves := make([][32]byte, len(actions))
	for i, a := range actions {
		h, _ := hexToHash(a.Hash)
		leaves[i] = h
	}
	return merkleProof(leaves, leafIndex)
}

func merkleProof(nodes [][32]byte, index int) []string {
	if len(nodes) <= 1 {
		return nil
	}
	if len(nodes)%2 != 0 {
		nodes = append(nodes, nodes[len(nodes)-1])
	}
	var label string
	var sibling [32]byte
	if index%2 == 0 {
		label = "right"
		sibling = nodes[index+1]
	} else {
		label = "left"
		sibling = nodes[index-1]
	}
	proof := []string{fmt.Sprintf("%s:%x", label, sibling)}
	var next [][32]byte
	for i := 0; i < len(nodes); i += 2 {
		combined := append(nodes[i][:], nodes[i+1][:]...)
		next = append(next, sha256.Sum256(combined))
	}
	return append(proof, merkleProof(next, index/2)...)
}

func hexToHash(h string) ([32]byte, error) {
	var out [32]byte
	h = strings.TrimPrefix(h, "sha256:")
	if len(h) < 64 {
		return out, fmt.Errorf("short hash")
	}
	for i := 0; i < 32; i++ {
		b := 0
		fmt.Sscanf(h[i*2:i*2+2], "%02x", &b)
		out[i] = byte(b)
	}
	return out, nil
}

// --- input helpers ---

func stringField(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func int64Field(m map[string]any, key string, def int64) int64 {
	switch v := m[key].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	}
	return def
}

func float64Field(m map[string]any, key string, def float64) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return def
}

func mapField(m map[string]any, key string) map[string]string {
	out := make(map[string]string)
	raw, ok := m[key].(map[string]any)
	if !ok {
		return out
	}
	for k, v := range raw {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}

func extractGitHubURL(input string) string {
	re := regexp.MustCompile(`https://github\.com/[\w\-.]+/[\w\-.]+`)
	match := re.FindString(input)
	return sanitizeGitHubURL(match)
}

func sanitizeGitHubURL(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.Trim(s, "\"'`")
	s = strings.TrimRight(s, ".,;:!?)]}>")
	s = strings.TrimSuffix(s, "/")
	s = strings.TrimSuffix(s, ".git")
	return s
}
