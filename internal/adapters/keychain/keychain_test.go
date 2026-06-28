package keychain

import (
	"testing"

	"github.com/zalando/go-keyring"
)

func TestStoreLoadDelete(t *testing.T) {
	keyring.MockInit() // in-memory keyring; no real OS keychain access

	const acct = "age1example"
	const secret = "AGE-SECRET-KEY-1example"

	if err := Store(acct, secret); err != nil {
		t.Fatal(err)
	}
	got, err := Load(acct)
	if err != nil || got != secret {
		t.Fatalf("Load = %q, %v; want %q", got, err, secret)
	}
	if err := Delete(acct); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(acct); err == nil {
		t.Fatal("expected error loading deleted entry")
	}
}
