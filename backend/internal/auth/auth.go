package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	nonceExpiry   = 10 * time.Minute
	nonceBytesLen = 16
)

type nonce struct {
	value     string
	expiresAt time.Time
}

// Service handles SIWE nonce management and JWT issuance.
type Service struct {
	jwtSecret []byte

	mu     sync.Mutex
	nonces map[string]nonce // address (lowercased) → nonce
}

// New creates a new auth service.
func New(jwtSecret string) *Service {
	return &Service{
		jwtSecret: []byte(jwtSecret),
		nonces:    make(map[string]nonce),
	}
}

// IssueNonce generates and stores a fresh nonce for the given wallet address.
func (s *Service) IssueNonce(address string) string {
	b := make([]byte, nonceBytesLen)
	_, _ = rand.Read(b)
	n := nonce{
		value:     hex.EncodeToString(b),
		expiresAt: time.Now().Add(nonceExpiry),
	}
	s.mu.Lock()
	s.nonces[strings.ToLower(address)] = n
	s.mu.Unlock()
	return n.value
}

// ConsumeNonce verifies that nonce matches the one issued for address and
// removes it (one-time use).
func (s *Service) ConsumeNonce(address, nonceValue string) error {
	key := strings.ToLower(address)
	s.mu.Lock()
	n, ok := s.nonces[key]
	if ok {
		delete(s.nonces, key)
	}
	s.mu.Unlock()

	if !ok {
		return fmt.Errorf("no nonce issued for address %s", address)
	}
	if time.Now().After(n.expiresAt) {
		return fmt.Errorf("nonce expired")
	}
	if n.value != nonceValue {
		return fmt.Errorf("nonce mismatch")
	}
	return nil
}

// IssueJWT creates a signed JWT for the given wallet address and team ID.
func (s *Service) IssueJWT(address, teamID string) (string, error) {
	claims := jwt.MapClaims{
		"sub":     strings.ToLower(address),
		"team_id": teamID,
		"iat":     time.Now().Unix(),
		"exp":     time.Now().Add(24 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

// ValidateJWT parses and validates a JWT, returning the address and team ID.
func (s *Service) ValidateJWT(tokenString string) (address, teamID string, err error) {
	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return "", "", fmt.Errorf("invalid token: %w", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return "", "", fmt.Errorf("invalid token claims")
	}
	address, _ = claims["sub"].(string)
	teamID, _ = claims["team_id"].(string)
	if address == "" {
		return "", "", fmt.Errorf("missing sub claim")
	}
	return address, teamID, nil
}

// GCNonces removes expired nonces. Call periodically (e.g., every minute).
func (s *Service) GCNonces() {
	now := time.Now()
	s.mu.Lock()
	for addr, n := range s.nonces {
		if now.After(n.expiresAt) {
			delete(s.nonces, addr)
		}
	}
	s.mu.Unlock()
}
