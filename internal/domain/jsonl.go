package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// MergeSessionJSONL combines two versions of a JSONL session file into the union
// of their records, so that divergent appends to the same session on two devices
// don't lose either side's history (whole-file last-writer-wins would).
//
// Each line is a session record; records are deduplicated by their "uuid" field
// when present, otherwise by exact content. The result keeps every unique record,
// preserving the order of a deterministically chosen base (the version with more
// records; ties broken by raw byte order) and appending the other side's records
// that the base lacks.
//
// The function is commutative (MergeSessionJSONL(a,b) and (b,a) produce identical
// bytes) and idempotent, so independent devices converge: once both hold the same
// set of records they compute the same file and stop rewriting.
func MergeSessionJSONL(a, b []byte) []byte {
	la := splitLines(a)
	lb := splitLines(b)

	// Deterministic base selection makes the merge order-independent.
	base, other := la, lb
	if len(lb) > len(la) || (len(lb) == len(la) && bytes.Compare(b, a) > 0) {
		base, other = lb, la
	}

	seen := make(map[string]struct{}, len(base)+len(other))
	out := make([][]byte, 0, len(base)+len(other))
	add := func(lines [][]byte) {
		for _, l := range lines {
			k := lineKey(l)
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			out = append(out, l)
		}
	}
	add(base)
	add(other)

	if len(out) == 0 {
		return nil
	}
	return append(bytes.Join(out, []byte("\n")), '\n')
}

// splitLines returns the non-blank lines of data, stripped of a trailing "\r",
// preserving order.
func splitLines(data []byte) [][]byte {
	raw := bytes.Split(data, []byte("\n"))
	out := make([][]byte, 0, len(raw))
	for _, line := range raw {
		line = bytes.TrimRight(line, "\r")
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		out = append(out, line)
	}
	return out
}

// lineKey identifies a record for deduplication: its session-record "uuid" when
// the line is a JSON object carrying one, else a content hash so lines without a
// uuid still dedupe exactly.
func lineKey(line []byte) string {
	var rec struct {
		UUID string `json:"uuid"`
	}
	if err := json.Unmarshal(line, &rec); err == nil && rec.UUID != "" {
		return "uuid:" + rec.UUID
	}
	sum := sha256.Sum256(line)
	return "hash:" + hex.EncodeToString(sum[:])
}
