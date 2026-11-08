package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// CanonicalJSON is the only canonicalization implementation in the backend. It
// turns any JSON document into the canonical bytes and SHA-256 hex digest that
// a request hash is compared by (spec §3.9.4, §6.4):
//
//   - input is decoded with plain encoding/json semantics (numbers become
//     float64, and for a duplicate object key the later value wins);
//   - object keys are sorted by byte order and the output is compact;
//   - HTML escaping is disabled and non-ASCII text stays raw UTF-8;
//   - floats use the shortest round-trip form (1.0 -> 1, 1.50 -> 1.5,
//     1e2 -> 100);
//   - no trailing newline is part of the hashed bytes.
//
// The digest is the lowercase hex SHA-256 of canonical. Both returned values
// are nil/empty when the input is not a single valid JSON document, in which
// case the error says why. Callers must not implement their own
// canonicalization; the future AI analysis submit handler uses this function.
func CanonicalJSON(input []byte) (canonical []byte, digest string, err error) {
	var value any
	if err := json.Unmarshal(input, &value); err != nil {
		return nil, "", fmt.Errorf("canonical json: decode: %w", err)
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, "", fmt.Errorf("canonical json: encode: %w", err)
	}

	// Encoder.Encode terminates the document with a newline; the hash covers
	// only the document itself.
	canonical = bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
	sum := sha256.Sum256(canonical)
	return canonical, hex.EncodeToString(sum[:]), nil
}
