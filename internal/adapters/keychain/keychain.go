// Package keychain stores the chain identity in the OS keychain (macOS Keychain,
// etc.) via go-keyring, so the secret never lands on disk in the repo or config.
package keychain

import "github.com/zalando/go-keyring"

// service is the keychain service name under which chain identities are stored.
const service = "ccsync"

// Store saves the identity secret under the given account (the chain recipient).
func Store(account, secret string) error {
	return keyring.Set(service, account, secret)
}

// Load returns the identity secret for an account.
func Load(account string) (string, error) {
	return keyring.Get(service, account)
}

// Delete removes the identity for an account.
func Delete(account string) error {
	return keyring.Delete(service, account)
}
