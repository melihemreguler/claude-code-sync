package domain

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// rec builds a one-line JSON session record with the given uuid.
func rec(uuid, text string) string {
	return `{"uuid":"` + uuid + `","text":"` + text + `"}`
}

func lines(data []byte) []string {
	s := strings.TrimRight(string(data), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func uuidSet(data []byte) map[string]bool {
	out := map[string]bool{}
	for _, l := range lines(data) {
		var r struct {
			UUID string `json:"uuid"`
		}
		_ = json.Unmarshal([]byte(l), &r)
		out[r.UUID] = true
	}
	return out
}

// TestMergeUnionNoLoss: divergent tails on both sides are both preserved.
func TestMergeUnionNoLoss(t *testing.T) {
	a := []byte(rec("1", "a") + "\n" + rec("2", "b") + "\n" + rec("3", "c") + "\n")
	b := []byte(rec("1", "a") + "\n" + rec("2", "b") + "\n" + rec("4", "d") + "\n")

	got := MergeSessionJSONL(a, b)
	set := uuidSet(got)
	for _, u := range []string{"1", "2", "3", "4"} {
		if !set[u] {
			t.Errorf("merged result missing record %q: %s", u, got)
		}
	}
	if len(lines(got)) != 4 {
		t.Errorf("expected 4 unique records, got %d: %s", len(lines(got)), got)
	}
}

// TestMergeDedupByUUID: same uuid with different payload is not duplicated.
func TestMergeDedupByUUID(t *testing.T) {
	a := []byte(rec("1", "original") + "\n")
	b := []byte(rec("1", "different-payload-same-uuid") + "\n")
	got := MergeSessionJSONL(a, b)
	if n := len(lines(got)); n != 1 {
		t.Fatalf("same uuid must not duplicate, got %d lines: %s", n, got)
	}
}

// TestMergeCommutative: order of arguments must not change the bytes.
func TestMergeCommutative(t *testing.T) {
	a := []byte(rec("1", "a") + "\n" + rec("3", "c") + "\n")
	b := []byte(rec("2", "b") + "\n" + rec("1", "a") + "\n" + rec("4", "d") + "\n")
	if !bytes.Equal(MergeSessionJSONL(a, b), MergeSessionJSONL(b, a)) {
		t.Errorf("merge is not commutative:\n ab=%s\n ba=%s", MergeSessionJSONL(a, b), MergeSessionJSONL(b, a))
	}
}

// TestMergeIdempotent: merging a result with one of its inputs is a no-op.
func TestMergeIdempotent(t *testing.T) {
	a := []byte(rec("1", "a") + "\n" + rec("2", "b") + "\n")
	b := []byte(rec("2", "b") + "\n" + rec("3", "c") + "\n")
	m := MergeSessionJSONL(a, b)
	if !bytes.Equal(MergeSessionJSONL(m, a), m) {
		t.Error("merge(merge(a,b), a) should equal merge(a,b)")
	}
	if !bytes.Equal(MergeSessionJSONL(m, m), m) {
		t.Error("merge(m, m) should equal m")
	}
}

// TestMergeConverges: two different orderings of the same record set converge to
// identical bytes after one cross-merge each (the cross-device steady state).
func TestMergeConverges(t *testing.T) {
	fa := []byte(rec("1", "a") + "\n" + rec("2", "b") + "\n" + rec("3", "c") + "\n")
	fb := []byte(rec("3", "c") + "\n" + rec("1", "a") + "\n" + rec("2", "b") + "\n")
	a2 := MergeSessionJSONL(fa, fb)
	b2 := MergeSessionJSONL(fb, fa)
	if !bytes.Equal(a2, b2) {
		t.Fatalf("devices did not converge:\n a=%s\n b=%s", a2, b2)
	}
}

// TestMergeSupersetAdoptsLonger: when one side is a superset, the result equals it.
func TestMergeSupersetAdoptsLonger(t *testing.T) {
	short := []byte(rec("1", "a") + "\n" + rec("2", "b") + "\n")
	long := []byte(rec("1", "a") + "\n" + rec("2", "b") + "\n" + rec("3", "c") + "\n")
	got := MergeSessionJSONL(short, long)
	if !bytes.Equal(got, long) {
		t.Errorf("superset should be adopted verbatim:\n got=%s\n want=%s", got, long)
	}
}

// TestMergeNoUUIDDedupByContent: lines without a uuid dedupe by exact content.
func TestMergeNoUUIDDedupByContent(t *testing.T) {
	a := []byte("plain line\n" + rec("1", "a") + "\n")
	b := []byte("plain line\n" + rec("2", "b") + "\n")
	got := MergeSessionJSONL(a, b)
	count := 0
	for _, l := range lines(got) {
		if l == "plain line" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("identical non-uuid lines should dedupe, found %d: %s", count, got)
	}
}

// TestMergeEmptyInputs: blank/empty handling.
func TestMergeEmptyInputs(t *testing.T) {
	if got := MergeSessionJSONL(nil, nil); got != nil {
		t.Errorf("merge of empties should be nil, got %q", got)
	}
	one := []byte(rec("1", "a") + "\n")
	if got := MergeSessionJSONL(nil, one); !bytes.Equal(got, one) {
		t.Errorf("merge(nil, x) should be x, got %s", got)
	}
}
