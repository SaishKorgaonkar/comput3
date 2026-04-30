// Package keeperhub provides an interface and implementations for KeeperHub
// job registration.  The agent calls RegisterJob at session completion so that
// an off-chain keeper can trigger the on-chain attestation submission.
package keeperhub

import (
	"context"
)

// Job is the payload submitted to KeeperHub.
type Job struct {
	Name        string `json:"name"`
	SessionID   string `json:"session_id"`
	Payload     any    `json:"payload"`
	CallbackURL string `json:"callback_url,omitempty"`
}

// Client is the interface used by the agent to manage keeper jobs.
type Client interface {
	RegisterJob(ctx context.Context, job Job) (string, error)
	CancelJob(ctx context.Context, jobID string) error
}

// NoopClient is a Client that silently discards all operations.
type NoopClient struct{}

func (NoopClient) RegisterJob(_ context.Context, _ Job) (string, error) { return "", nil }
func (NoopClient) CancelJob(_ context.Context, _ string) error           { return nil }

// New returns a KeeperHub client if credentials are provided, otherwise a NoopClient.
func New(endpoint, privateKey string) (Client, error) {
	if endpoint == "" || privateKey == "" {
		return NoopClient{}, nil
	}
	// TODO: implement real KeeperHub client using HTTP + HMAC auth.
	return NoopClient{}, nil
}
