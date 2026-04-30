package chain

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// DeriveVaultKey computes HMAC-SHA256(masterSecret, containerID) and returns
// the result as a 64-character hex string (AES-256 key).
func DeriveVaultKey(masterSecret, containerID string) string {
	mac := hmac.New(sha256.New, []byte(masterSecret))
	mac.Write([]byte(containerID))
	return hex.EncodeToString(mac.Sum(nil))
}

// HexToBytes32 decodes a 64-character hex string into a [32]byte.
func HexToBytes32(h string) ([32]byte, error) {
	var out [32]byte
	h = strings.TrimPrefix(h, "0x")
	b, err := hex.DecodeString(h)
	if err != nil {
		return out, fmt.Errorf("HexToBytes32: %w", err)
	}
	if len(b) != 32 {
		return out, fmt.Errorf("HexToBytes32: expected 32 bytes, got %d", len(b))
	}
	copy(out[:], b)
	return out, nil
}
