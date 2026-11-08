package api

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// canonicalVectors are the fixed golden vectors for CanonicalJSON: each case
// carries the exact canonical bytes and the SHA-256 hex digest they hash to.
// The digests were produced outside the implementation, so a formatting change
// in the helper fails the table instead of being accepted silently.
var canonicalVectors = []struct {
	name      string
	input     string
	canonical string
	digest    string
}{
	{
		name:      "object keys sorted by byte order",
		input:     `{"b":1,"a":2}`,
		canonical: `{"a":2,"b":1}`,
		digest:    "d3626ac30a87e6f7a6428233b3c68299976865fa5508e4267c5415c76af7a772",
	},
	{
		name:      "whitespace and newlines are removed",
		input:     "{\n  \"a\" : 2,\n  \"b\": 1\n}",
		canonical: `{"a":2,"b":1}`,
		digest:    "d3626ac30a87e6f7a6428233b3c68299976865fa5508e4267c5415c76af7a772",
	},
	{
		name:      "duplicate key later value wins",
		input:     `{"a":1,"a":2}`,
		canonical: `{"a":2}`,
		digest:    "7e8059f495589fcd981232cc11d00b00da3802c01d688fa1cf1f6bed6e5bb33c",
	},
	{
		name:      "HTML characters are not escaped",
		input:     `{"html":"\u003ca\u003e \u0026 \u003c/a\u003e"}`,
		canonical: `{"html":"<a> & </a>"}`,
		digest:    "d50d1f48726d9745007c2e2f9ff2ebe951ccd39e86037702ae11218e346439e0",
	},
	{
		name:      "Japanese text stays raw UTF-8",
		input:     `{"text":"日本語"}`,
		canonical: `{"text":"日本語"}`,
		digest:    "75cbb8b1d961a7c996bbf914f5d3c8d0793cb116be763162684a3d026951471b",
	},
	{
		name:      "floats use shortest round-trip form",
		input:     `{"one":1.0,"oneAndHalf":1.50,"tenth":0.1,"hundred":1e2}`,
		canonical: `{"hundred":100,"one":1,"oneAndHalf":1.5,"tenth":0.1}`,
		digest:    "4b4697eaa653e7a117687c9dcbf3d24bab3b5666d40463f672ef4c7965ed8e87",
	},
	{
		name:      "nested objects inside arrays keep array order",
		input:     `{"items":[{"z":1,"y":2},{"b":[3,{"d":4,"c":5}]}]}`,
		canonical: `{"items":[{"y":2,"z":1},{"b":[3,{"c":5,"d":4}]}]}`,
		digest:    "bb76dcfee5db3e408d4fe4ef427ea4214f25d5b18a88efe0fb088aafec34a753",
	},
	{
		name:      "null and booleans",
		input:     `{"n":null,"t":true,"f":false}`,
		canonical: `{"f":false,"n":null,"t":true}`,
		digest:    "22e00dc2f7b01420f940fbdbfbdf34fa0667cc6500186495023ba37722cbd05e",
	},
	{
		name: "full spec sample request shape",
		input: `{
  "submission_id": "11111111-1111-1111-1111-111111111111",
  "activity_id": "22222222-2222-2222-2222-222222222222",
  "transcripts": [
    {
      "user_id": "33333333-3333-3333-3333-333333333333",
      "content": "こんにちは",
      "accent": null,
      "start_offset_sec": 0.0,
      "end_offset_sec": 1.5
    }
  ],
  "mistakes": [
    {
      "user_id": "33333333-3333-3333-3333-333333333333",
      "type": "grammar",
      "origin_text": "私は",
      "fixed_text": "わたしは",
      "confidence": "high",
      "start_offset_sec": 0.0,
      "end_offset_sec": 1.5,
      "comment": null
    }
  ]
}`,
		canonical: `{"activity_id":"22222222-2222-2222-2222-222222222222","mistakes":[{"comment":null,"confidence":"high","end_offset_sec":1.5,"fixed_text":"わたしは","origin_text":"私は","start_offset_sec":0,"type":"grammar","user_id":"33333333-3333-3333-3333-333333333333"}],"submission_id":"11111111-1111-1111-1111-111111111111","transcripts":[{"accent":null,"content":"こんにちは","end_offset_sec":1.5,"start_offset_sec":0,"user_id":"33333333-3333-3333-3333-333333333333"}]}`,
		digest:    "a1d6e2ae1dd6e4c48a3ce7aad308075222226035a5a6c015ec4648e7c6483297",
	},
}

func TestCanonicalJSONVectors(t *testing.T) {
	for _, tc := range canonicalVectors {
		t.Run(tc.name, func(t *testing.T) {
			canonical, digest, err := CanonicalJSON([]byte(tc.input))
			require.NoError(t, err)
			assert.Equal(t, tc.canonical, string(canonical))
			assert.Equal(t, tc.digest, digest)
			assert.Len(t, digest, sha256.Size*2)

			// The digest covers the canonical bytes, not the input.
			sum := sha256.Sum256(canonical)
			assert.Equal(t, hex.EncodeToString(sum[:]), digest)
		})
	}
}

func TestCanonicalJSONEquivalentInputsShareADigest(t *testing.T) {
	equivalent := []string{
		`{"a":2,"b":1}`,
		`{"b":1,"a":2}`,
		"{\n  \"a\": 2,\n  \"b\": 1\n}",
		" \t{\"b\" : 1, \"a\" : 2}\r\n",
	}

	var want string
	for i, input := range equivalent {
		_, digest, err := CanonicalJSON([]byte(input))
		require.NoError(t, err, "input %d", i)
		if i == 0 {
			want = digest
			continue
		}
		assert.Equal(t, want, digest, "input %d", i)
	}

	_, duplicateDigest, err := CanonicalJSON([]byte(`{"a":1,"a":2}`))
	require.NoError(t, err)
	_, literalDigest, err := CanonicalJSON([]byte(`{"a":2}`))
	require.NoError(t, err)
	assert.Equal(t, literalDigest, duplicateDigest)
}

func TestCanonicalJSONDifferentPayloadsDifferentDigests(t *testing.T) {
	digests := make(map[string]string)
	for name, input := range map[string]string{
		"base":                 `{"a":2,"b":1}`,
		"changed value":        `{"a":3,"b":1}`,
		"array order one":      `{"items":[1,2,3]}`,
		"array order two":      `{"items":[3,2,1]}`,
		"added key":            `{"a":2,"b":1,"c":null}`,
		"different value type": `{"a":"2","b":1}`,
	} {
		_, digest, err := CanonicalJSON([]byte(input))
		require.NoError(t, err, name)
		digests[name] = digest
	}

	assert.NotEqual(t, digests["base"], digests["changed value"])
	assert.NotEqual(t, digests["array order one"], digests["array order two"])
	assert.NotEqual(t, digests["base"], digests["added key"])
	assert.NotEqual(t, digests["base"], digests["different value type"])
}

func TestCanonicalJSONRejectsInvalidInput(t *testing.T) {
	for _, input := range []string{
		"",
		"not json",
		"{",
		`{"a":}`,
		`{"a":1,}`,
		`{"a":1} {"b":2}`,
		`{"a":NaN}`,
	} {
		t.Run(input, func(t *testing.T) {
			canonical, digest, err := CanonicalJSON([]byte(input))
			require.Error(t, err)
			assert.Nil(t, canonical)
			assert.Empty(t, digest)
		})
	}
}
