package blobstoretest

import "testing"

// The in-memory reference must satisfy the contract; this also runs the contract
// body on every CI build (the real backends only run it with live credentials).
func TestMemSatisfiesContract(t *testing.T) {
	Run(t, NewMem())
}
