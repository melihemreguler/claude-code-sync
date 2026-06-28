// Package nocrypto provides a passthrough ports.Crypto implementation. It exists
// so the sync core already routes payloads through Seal/Open; P2 swaps in age
// encryption without changing the core.
package nocrypto

// Passthrough returns payloads unchanged.
type Passthrough struct{}

// Seal returns the plaintext unchanged.
func (Passthrough) Seal(plaintext []byte) ([]byte, error) { return plaintext, nil }

// Open returns the ciphertext unchanged.
func (Passthrough) Open(ciphertext []byte) ([]byte, error) { return ciphertext, nil }
