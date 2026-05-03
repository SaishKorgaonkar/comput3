package scanner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// StackComponent is a detected technology component.
type StackComponent struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Type    string `json:"type"` // "frontend" | "backend" | "database" | "smart-contract" | "infra"
}

// ContainerSpec is a prescription for a single container.
type ContainerSpec struct {
	Name     string            `json:"name"`
	Image    string            `json:"image"`
	Ports    []string          `json:"ports"`
	RAMMb    int64             `json:"ram_mb"`
	CPUCores float64           `json:"cpu_cores"`
	EnvVars  map[string]string `json:"env_vars"`
}

// DeploymentPlan is the output of a repository scan.
type DeploymentPlan struct {
	RepoURL              string           `json:"repo_url"`
	DetectedStack        []StackComponent `json:"detected_stack"`
	Containers           []ContainerSpec  `json:"containers"`
	HasSmartContracts    bool             `json:"has_smart_contracts"`
	RecommendedNetwork   string           `json:"recommended_network"`
	EstimatedCostPerHour float64          `json:"estimated_cost_per_hour"`
	Summary              string           `json:"summary"`
	DeploymentSteps      []string         `json:"deployment_steps"`
}

// Scanner uses Groq to analyze repos and produce deployment plans.
type Scanner struct {
	apiKey string
	model  string
}

// New returns a Scanner backed by the Groq API (OpenAI-compatible).
func New(apiKey, model string) *Scanner {
	if model == "" {
		model = "llama-3.3-70b-versatile"
	}
	return &Scanner{apiKey: apiKey, model: model}
}

// AnalyzeRepo clones the repo and returns a deployment plan.
func (s *Scanner) AnalyzeRepo(ctx context.Context, repoURL string) (*DeploymentPlan, error) {
	repoURL = sanitizeGitHubURL(repoURL)
	dir, err := os.MkdirTemp("", "comput3-scan-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	cloneURL := extractRepoRoot(repoURL)
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth=1", "--quiet", cloneURL, dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("clone %s: %w\n%s", repoURL, err, string(out))
	}

	files, err := collectFiles(dir)
	if err != nil {
		return nil, fmt.Errorf("collect files: %w", err)
	}

	return s.analyzeWithGroq(ctx, repoURL, files)
}

type repoFile struct {
	Path    string
	Content string
}

func collectFiles(root string) ([]repoFile, error) {
	priority := []string{
		"package.json", "requirements.txt", "pyproject.toml", "go.mod",
		"Dockerfile", "docker-compose.yml", "docker-compose.yaml",
		"hardhat.config.js", "hardhat.config.ts", "foundry.toml",
		"next.config.js", "next.config.ts",
		"vite.config.js", "vite.config.ts",
		".env.example", ".env.sample", "README.md",
	}

	seen := map[string]bool{}
	var files []repoFile

	for _, name := range priority {
		content, err := readTruncated(filepath.Join(root, name), 4000)
		if err == nil {
			files = append(files, repoFile{Path: name, Content: content})
			seen[name] = true
		}
	}

	solCount := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			n := d.Name()
			if strings.HasPrefix(n, ".") || n == "node_modules" ||
				n == "dist" || n == "build" || n == ".next" ||
				n == "out" || n == "target" || n == "artifacts" {
				return filepath.SkipDir
			}
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if seen[rel] || strings.Count(rel, "/") > 2 {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		base := filepath.Base(path)

		if ext == ".sol" && solCount < 5 {
			if content, err := readTruncated(path, 3000); err == nil {
				files = append(files, repoFile{Path: rel, Content: content})
				seen[rel] = true
				solCount++
			}
		}
		if base == "package.json" || base == "hardhat.config.js" ||
			base == "hardhat.config.ts" || base == "foundry.toml" ||
			base == "requirements.txt" || base == "go.mod" {
			if content, err := readTruncated(path, 3000); err == nil {
				files = append(files, repoFile{Path: rel, Content: content})
				seen[rel] = true
			}
		}
		return nil
	})
	return files, err
}

func (s *Scanner) analyzeWithGroq(ctx context.Context, repoURL string, files []repoFile) (*DeploymentPlan, error) {
	var sb strings.Builder
	for _, f := range files {
		sb.WriteString(fmt.Sprintf("\n\n### FILE: %s\n```\n%s\n```", f.Path, f.Content))
	}

	prompt := fmt.Sprintf(`You are a deployment analyzer for COMPUT3, a decentralized trustless compute platform for AI agents.

Analyze the repository files below and return a JSON deployment plan.

Repository: %s

Files:
%s

Return ONLY valid JSON matching this exact schema (no markdown, no explanation):
{
  "repo_url": "string",
  "detected_stack": [
    {"name": "string", "version": "string", "type": "frontend|backend|database|smart-contract|infra"}
  ],
  "containers": [
    {
      "name": "string",
      "image": "official Docker image tag",
      "ports": ["3000/tcp"],
      "ram_mb": 2048,
      "cpu_cores": 1.0,
      "env_vars": {"KEY": "value or empty string"}
    }
  ],
  "has_smart_contracts": false,
  "recommended_network": "base-sepolia or mainnet or none",
  "estimated_cost_per_hour": 0.05,
  "summary": "one-sentence description",
  "deployment_steps": ["step1", "step2"]
}

Rules:
- Use minimal official images (node:20-alpine, python:3.12-slim, golang:1.22-alpine)
- Separate containers for databases (postgres:16-alpine, mongo:7, redis:7-alpine)
- Next.js: node:20-alpine, expose 3000/tcp
- Hardhat/Foundry: set has_smart_contracts=true
- RAM: 512MB databases, 1024-2048MB apps
- estimated_cost_per_hour: $0.02-0.10 based on total resources`, repoURL, sb.String())

	// Call Groq via OpenAI-compatible chat completions API
	reqBody := map[string]any{
		"model":      s.model,
		"max_tokens": 4096,
		"messages": []map[string]any{
			{"role": "user", "content": prompt},
		},
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.groq.com/openai/v1/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("groq api: %w", err)
	}
	defer resp.Body.Close()

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode groq response: %w", err)
	}
	if apiResp.Error != nil {
		return nil, fmt.Errorf("groq error: %s", apiResp.Error.Message)
	}
	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("groq: empty choices")
	}

	rawJSON := strings.TrimSpace(apiResp.Choices[0].Message.Content)
	if strings.HasPrefix(rawJSON, "```") {
		lines := strings.Split(rawJSON, "\n")
		if len(lines) > 2 {
			rawJSON = strings.Join(lines[1:len(lines)-1], "\n")
		}
	}

	var plan DeploymentPlan
	if err := json.Unmarshal([]byte(rawJSON), &plan); err != nil {
		return nil, fmt.Errorf("parse deployment plan JSON: %w\nraw: %s", err, rawJSON)
	}
	plan.RepoURL = repoURL
	return &plan, nil
}

// --- helpers ---

func extractRepoRoot(rawURL string) string {
	rawURL = sanitizeGitHubURL(rawURL)
	parts := strings.SplitN(rawURL, "/", 6)
	if len(parts) >= 5 && strings.Contains(rawURL, "github.com") {
		return strings.Join(parts[:5], "/")
	}
	return rawURL
}

func sanitizeGitHubURL(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.Trim(s, "\"'`")
	s = strings.TrimRight(s, ".,;:!?)]}>")
	s = strings.TrimSuffix(s, "/")
	s = strings.TrimSuffix(s, ".git")
	return s
}

func readTruncated(path string, maxBytes int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	buf := make([]byte, maxBytes)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}
	content := string(buf[:n])
	if n == maxBytes {
		content += "\n... (truncated)"
	}
	return content, nil
}
