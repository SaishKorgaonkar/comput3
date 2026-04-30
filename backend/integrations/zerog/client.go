// Package zerog provides an interface and implementations for 0G Network
// KV/log storage.  When the 0G endpoint is not configured, the Noop
// implementation is used so the agent functions normally without the external
// service.
package zerog

import (
	"context"
)

// Client is the interface used by the agent to persist session actions.
type Client interface {
	// Put stores a key-value pair in 0G Network KV storage.
	Put(ctx context.Context, key string, value []byte) error
	// Get retrieves a value by key.
	Get(ctx context.Context, key string) ([]byte, error)
	// Append appends a byte slice to the session's append-only log.
	Append(ctx context.Context, logID string, entry []byte) error
	// ReadLog retrieves all previously appended entries for a session.
	ReadLog(ctx context.Context, logID string) ([][]byte, error)
}

// NoopClient is a Client that silently discards all data.
type NoopClient struct{}

func (NoopClient) Put(_ context.Context, _ string, _ []byte) error          { return nil }
func (NoopClient) Get(_ context.Context, _ string) ([]byte, error)          { return nil, nil }
func (NoopClient) Append(_ context.Context, _ string, _ []byte) error       { return nil }
func (NoopClient) ReadLog(_ context.Context, _ string) ([][]byte, error)    { return nil, nil }

// New returns a 0G client if credentials are provided, otherwise a NoopClient.
// A real implementation would connect to the 0G Flow contract and KV node.
func New(rpcURL, privateKey, flowAddress string) (Client, error) {
	if rpcURL == "" || privateKey == "" || flowAddress == "" {
		return NoopClient{}, nil
	}
	// TODO: implement real 0G Network client using the 0G SDK.
	return NoopClient{}, nil
}
