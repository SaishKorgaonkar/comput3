// Package axl provides an interface and implementations for AXL (Gensyn)
// pub/sub messaging.  Topic naming follows the comput3 convention:
// comput3.session.<sessionID>.
package axl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is the interface used by the agent to publish/subscribe events.
type Client interface {
	// Publish sends msg to the given topic.
	Publish(ctx context.Context, topic string, msg []byte) error
	// Subscribe registers handler for messages on topic.
	Subscribe(ctx context.Context, topic string, handler func(msg []byte)) error
}

// NoopClient is a Client that silently discards all messages.
type NoopClient struct{}

func (NoopClient) Publish(_ context.Context, _ string, _ []byte) error         { return nil }
func (NoopClient) Subscribe(_ context.Context, _ string, _ func([]byte)) error { return nil }

// httpClient is an AXL client that communicates with the Gensyn AXL REST API.
// POST /api/v1/publish — publish a message to a topic.
// GET  /api/v1/subscribe?topic=<topic>&from=<offset> — long-poll for messages.
type httpClient struct {
	endpoint   string
	privateKey string
	http       *http.Client
}

type publishRequest struct {
	Topic   string `json:"topic"`
	Payload string `json:"payload"` // base64-encoded message bytes
}

type subscribeResponse struct {
	Messages []struct {
		Payload string `json:"payload"` // base64-encoded
		Offset  int64  `json:"offset"`
	} `json:"messages"`
}

func (c *httpClient) Publish(ctx context.Context, topic string, msg []byte) error {
	body, _ := json.Marshal(publishRequest{
		Topic:   topic,
		Payload: fmt.Sprintf("%x", msg), // hex-encoded for simplicity
	})
	url := strings.TrimRight(c.endpoint, "/") + "/api/v1/publish"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("axl Publish: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.privateKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("axl Publish: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("axl Publish: server error %d: %s", resp.StatusCode, raw)
	}
	return nil
}

// Subscribe polls the AXL endpoint for new messages on the topic in a goroutine.
// The goroutine exits when ctx is cancelled.
func (c *httpClient) Subscribe(ctx context.Context, topic string, handler func(msg []byte)) error {
	go func() {
		var offset int64
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			url := fmt.Sprintf("%s/api/v1/subscribe?topic=%s&from=%d",
				strings.TrimRight(c.endpoint, "/"), topic, offset)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				time.Sleep(2 * time.Second)
				continue
			}
			req.Header.Set("Authorization", "Bearer "+c.privateKey)
			resp, err := c.http.Do(req)
			if err != nil {
				time.Sleep(2 * time.Second)
				continue
			}
			var result subscribeResponse
			_ = json.NewDecoder(resp.Body).Decode(&result)
			resp.Body.Close()
			for _, m := range result.Messages {
				// Decode hex payload
				var decoded []byte
				fmt.Sscanf(m.Payload, "%x", &decoded)
				if len(decoded) > 0 {
					handler(decoded)
				}
				if m.Offset >= offset {
					offset = m.Offset + 1
				}
			}
			if len(result.Messages) == 0 {
				time.Sleep(500 * time.Millisecond)
			}
		}
	}()
	return nil
}

// New returns an AXL client if credentials are provided, otherwise a NoopClient.
func New(endpoint, privateKey string) (Client, error) {
	if endpoint == "" || privateKey == "" {
		return NoopClient{}, nil
	}
	return &httpClient{
		endpoint:   endpoint,
		privateKey: privateKey,
		http:       &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// SessionTopic returns the canonical topic name for a session.
func SessionTopic(sessionID string) string {
	return "comput3.session." + sessionID
}
