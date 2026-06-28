// Package agecrypto implements ports.Crypto with age (filippo.io/age) X25519
// encryption. A single chain identity (private key) decrypts what is encrypted to
// its recipient (public key); every device in the chain holds the same identity.
package agecrypto

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"filippo.io/age"
)

// Crypto seals to / opens from a single chain identity.
type Crypto struct {
	identity  *age.X25519Identity
	recipient *age.X25519Recipient
	nameKey   []byte // HMAC key for opaque object names, derived from the identity
}

// New builds a Crypto from an age identity string ("AGE-SECRET-KEY-1…").
func New(identityStr string) (*Crypto, error) {
	id, err := age.ParseX25519Identity(identityStr)
	if err != nil {
		return nil, fmt.Errorf("invalid chain identity: %w", err)
	}
	nameKey := sha256.Sum256([]byte("ccsync/object-name\x00" + identityStr))
	return &Crypto{identity: id, recipient: id.Recipient(), nameKey: nameKey[:]}, nil
}

// HashName returns an HMAC of name keyed by a secret derived from the chain
// identity, so object directory names cannot be precomputed from known repo names.
func (c *Crypto) HashName(name string) string {
	mac := hmac.New(sha256.New, c.nameKey)
	mac.Write([]byte(name))
	return hex.EncodeToString(mac.Sum(nil))
}

// Generate creates a fresh chain identity, returning the secret identity string
// and its public recipient string.
func Generate() (identity string, recipient string, err error) {
	id, err := age.GenerateX25519Identity()
	if err != nil {
		return "", "", err
	}
	return id.String(), id.Recipient().String(), nil
}

// RecipientFromIdentity derives the public recipient string from an identity.
func RecipientFromIdentity(identityStr string) (string, error) {
	id, err := age.ParseX25519Identity(identityStr)
	if err != nil {
		return "", fmt.Errorf("invalid chain identity: %w", err)
	}
	return id.Recipient().String(), nil
}

// Seal encrypts plaintext to the chain recipient.
func (c *Crypto) Seal(plaintext []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, c.recipient)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(plaintext); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Open decrypts ciphertext with the chain identity.
func (c *Crypto) Open(ciphertext []byte) ([]byte, error) {
	r, err := age.Decrypt(bytes.NewReader(ciphertext), c.identity)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(r)
}
