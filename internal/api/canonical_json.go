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
//   - U+2028 LINE SEPARATOR and U+2029 PARAGRAPH SEPARATOR stay raw UTF-8 too,
//     in values and in object keys, although encoding/json escapes them even
//     with HTML escaping disabled (see rawLineSeparators);
//   - floats use the shortest round-trip form (1.0 -> 1, 1.50 -> 1.5,
//     1e2 -> 100);
//   - no trailing newline is part of the hashed bytes;
//   - invalid UTF-8 inside a string is replaced by U+FFFD, as encoding/json does,
//     so two payloads that differ only in such bytes share a digest.
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
	canonical = rawLineSeparators(bytes.TrimSuffix(buf.Bytes(), []byte("\n")))
	sum := sha256.Sum256(canonical)
	return canonical, hex.EncodeToString(sum[:]), nil
}

// rawLineSeparators rewrites the two escapes that encoding/json always writes,
// even with HTML escaping disabled, into the raw UTF-8 bytes they stand for:
//
//	\u2028 -> U+2028 LINE SEPARATOR      (e2 80 a8)
//	\u2029 -> U+2029 PARAGRAPH SEPARATOR (e2 80 a9)
//
// encoding/json escapes both unconditionally because they break JSONP
// (https://en.wikipedia.org/wiki/JSON#Safety); the canonical form this package
// promises keeps non-ASCII text as raw UTF-8, so it must undo them.
//
// The rewrite is escape aware. Text that literally contains the six characters
// \u2028 reaches the encoder as a backslash, so the encoder writes it as
// \\u2028. Replacing the byte sequence \u2028 blindly would match the tail of
// \\u2028 and corrupt that text, so this function copies every escape sequence
// as a unit and only rewrites an escape whose backslash starts the sequence.
// Runs of escaped backslashes before u2028 follow the same rule: each \\ pair is
// copied whole, so the part after it is never read as a separator escape.
//
// encoded must be the compact output of a json.Encoder with HTML escaping
// disabled: the only four-digit escapes such an encoder writes inside a string
// are \u00XX for control characters, \ufffd for invalid UTF-8, and the two
// separators rewritten here. Strings are the only place where a backslash
// occurs, so the scan tracks string boundaries and copies every byte outside a
// string unchanged.
func rawLineSeparators(encoded []byte) []byte {
	out := make([]byte, 0, len(encoded))
	inString := false
	for i := 0; i < len(encoded); {
		c := encoded[i]
		switch {
		case c == '"':
			inString = !inString
			out = append(out, c)
			i++
		case !inString || c != '\\':
			out = append(out, c)
			i++
		case isLineSeparatorEscape(encoded[i:]):
			if encoded[i+5] == '8' {
				out = append(out, "\u2028"...)
			} else {
				out = append(out, "\u2029"...)
			}
			i += 6
		default:
			// Another escape: copy the backslash and its introducer together,
			// so an escaped backslash never starts a separator match.
			n := min(2, len(encoded)-i)
			out = append(out, encoded[i:i+n]...)
			i += n
		}
	}
	return out
}

// isLineSeparatorEscape reports whether rest starts with the six bytes of the
// \u2028 or \u2029 escape.
func isLineSeparatorEscape(rest []byte) bool {
	return len(rest) >= 6 &&
		rest[0] == '\\' && rest[1] == 'u' &&
		rest[2] == '2' && rest[3] == '0' && rest[4] == '2' &&
		(rest[5] == '8' || rest[5] == '9')
}
