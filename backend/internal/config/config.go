package config

import "os"

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	Port        string
	DatabaseURL string
	DockerHost  string

	// LLM — Claude is the primary model for both scanning and agent
	AnthropicAPIKey string
	ScanModel       string // Claude model for repo scanning
	AgentModel      string // Claude model for deployment agent

	// Deploy domain for subdomain proxy (e.g. "deploy.comput3.xyz")
	DeployDomain string

	// Blockchain — Ethereum Sepolia
	EthSepolia_RPC_URL      string
	ProviderRegistryAddress string
	DeploymentEscrowAddress string
	JobAuctionAddress       string
	EASSchemaUID            string
	AgentWalletPrivateKey   string

	// ProviderMode: if true, this node watches for JobPosted events and auto-bids.
	ProviderMode              bool
	ProviderWalletPrivateKey  string

	// Vault — HMAC master secret for per-container LUKS key derivation
	VaultMasterSecret string

	// JWTSecret — signs wallet auth tokens
	JWTSecret string

	// 0G Network — decentralized agent memory
	ZeroG_RPC_URL      string
	ZeroG_PrivateKey   string
	ZeroG_FlowAddress  string

	// Gensyn AXL — cross-node agent pub/sub
	// AXL_Endpoint is the local AXL node API URL (e.g. http://127.0.0.1:9002).
	// AXL_PeerID is the destination peer's 64-char hex ed25519 public key.
	AXL_Endpoint string
	AXL_PeerID   string

	// KeeperHub — on-chain execution reliability
	KeeperHub_Endpoint   string
	KeeperHub_PrivateKey string
}

// Load reads all configuration from environment variables with sensible defaults.
func Load() *Config {
	return &Config{
		Port:        getEnv("PORT", "8080"),
		DatabaseURL: getEnv("DATABASE_URL", "postgres://comput3:comput3@localhost:5432/comput3?sslmode=disable"),
		DockerHost:  getEnv("DOCKER_HOST", "unix:///var/run/docker.sock"),

		AnthropicAPIKey: getEnv("ANTHROPIC_API_KEY", ""),
		ScanModel:       getEnv("SCAN_MODEL", "claude-3-5-haiku-20241022"),
		AgentModel:      getEnv("AGENT_MODEL", "claude-opus-4-5"),

		DeployDomain: getEnv("DEPLOY_DOMAIN", ""),

		EthSepolia_RPC_URL:      getEnv("ETH_SEPOLIA_RPC_URL", "https://rpc.sepolia.org"),
		ProviderRegistryAddress: getEnv("PROVIDER_REGISTRY_ADDRESS", ""),
		DeploymentEscrowAddress: getEnv("DEPLOYMENT_ESCROW_ADDRESS", ""),
		JobAuctionAddress:       getEnv("JOB_AUCTION_ADDRESS", ""),
		EASSchemaUID:            getEnv("EAS_SCHEMA_UID", ""),
		AgentWalletPrivateKey:   getEnv("AGENT_WALLET_PRIVATE_KEY", ""),

		ProviderMode:             getEnv("PROVIDER_MODE", "") == "true",
		ProviderWalletPrivateKey: getEnv("PROVIDER_WALLET_PRIVATE_KEY", ""),
		VaultMasterSecret:       getEnv("VAULT_MASTER_SECRET", ""),
		JWTSecret:               getEnv("JWT_SECRET", getEnv("VAULT_MASTER_SECRET", "comput3-dev-secret")),

		ZeroG_RPC_URL:     getEnv("ZG_RPC_URL", ""),
		ZeroG_PrivateKey:  getEnv("ZG_PRIVATE_KEY", ""),
		ZeroG_FlowAddress: getEnv("ZG_FLOW_ADDRESS", ""),

		AXL_Endpoint: getEnv("AXL_ENDPOINT", ""),
		AXL_PeerID:   getEnv("AXL_PEER_ID", ""),

		KeeperHub_Endpoint:   getEnv("KEEPERHUB_ENDPOINT", ""),
		KeeperHub_PrivateKey: getEnv("KEEPERHUB_PRIVATE_KEY", ""),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
