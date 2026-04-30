package chain

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// DeriveVaultKey computes HMAC-SHA256(masterSecret, containerID) and returns
// the result as a 64-character hex string (AES-256 key).
//
// The provider never sees the raw masterSecret — only the per-container derived key.
func DeriveVaultKey(masterSecret, containerID string) string {
	mac := hmac.New(sha256.New, []byte(masterSecret))
	mac.Write([]byte(containerID))
	return hex.EncodeToString(mac.Sum(nil))
}
