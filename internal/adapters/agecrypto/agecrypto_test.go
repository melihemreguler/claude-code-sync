package agecrypto

import (
	"bytes"
	"testing"
)

func TestSealOpenRoundTrip(t *testing.T) {
	idStr, _, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	c, err := New(idStr)
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte(`{"type":"user","cwd":"/Users/me/secret"}`)
	sealed, err := c.Seal(plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("secret")) {
		t.Fatal("ciphertext leaks plaintext")
	}
	out, err := c.Open(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, plain) {
		t.Fatalf("round trip mismatch: %q", out)
	}
}

func TestOpenRejectsTamperedCiphertext(t *testing.T) {
	idStr, _, _ := Generate()
	c, _ := New(idStr)
	sealed, _ := c.Seal([]byte("hello"))
	sealed[len(sealed)-1] ^= 0xff // flip last byte
	if _, err := c.Open(sealed); err == nil {
		t.Fatal("expected error opening tampered ciphertext")
	}
}

func TestOpenRejectsWrongIdentity(t *testing.T) {
	idA, _, _ := Generate()
	idB, _, _ := Generate()
	ca, _ := New(idA)
	cb, _ := New(idB)
	sealed, _ := ca.Seal([]byte("hello"))
	if _, err := cb.Open(sealed); err == nil {
		t.Fatal("a different identity must not decrypt")
	}
}

func TestRecipientFromIdentityStable(t *testing.T) {
	idStr, rec, _ := Generate()
	got, err := RecipientFromIdentity(idStr)
	if err != nil || got != rec {
		t.Fatalf("recipient mismatch: %q vs %q (err %v)", got, rec, err)
	}
}
