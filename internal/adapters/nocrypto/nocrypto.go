// Package nocrypto provides a passthrough ports.Crypto implementation for tests
// and tooling. It does not encrypt; object names fall back to a plain hash.
package nocrypto

import "github.com/melihemreguler/claude-code-sync/internal/domain"

// Passthrough returns payloads unchanged.
type Passthrough struct{}

// Seal returns the plaintext unchanged.
func (Passthrough) Seal(plaintext []byte) ([]byte, error) { return plaintext, nil }

// Open returns the ciphertext unchanged.
func (Passthrough) Open(ciphertext []byte) ([]byte, error) { return ciphertext, nil }

// HashName returns the plain (unkeyed) hash of the key.
func (Passthrough) HashName(name string) string {
	return domain.KeyHash(domain.CanonicalKey(name))
}
