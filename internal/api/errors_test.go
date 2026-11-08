package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"jpcorrect-backend/internal/domain"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// catalogRow is one documented code: the sentinel that raises it, its HTTP
// status and the string the envelope carries.
type catalogRow struct {
	code     string
	status   int
	sentinel error
}

// specCatalog is the §1.5 business code table, written out by hand so this test
// fails if the runtime mapping drifts from the specification.
var specCatalog = []catalogRow{
	{"USER_NOT_INITIALIZED", http.StatusUnauthorized, domain.ErrUserNotInitialized},
	{"GUILD_LIMIT_REACHED", http.StatusConflict, domain.ErrGuildLimitReached},
	{"GUILD_JOIN_LIMIT_REACHED", http.StatusConflict, domain.ErrGuildJoinLimitReached},
	{"ALREADY_GUILD_MEMBER", http.StatusConflict, domain.ErrAlreadyGuildMember},
	{"PENDING_APPLICATION_EXISTS", http.StatusConflict, domain.ErrPendingApplicationExists},
	{"INVITE_INVALID", http.StatusNotFound, domain.ErrInviteInvalid},
	{"SELECTION_LIMIT_REACHED", http.StatusConflict, domain.ErrSelectionLimitReached},
	{"NO_SELECTED_SENTENCE", http.StatusConflict, domain.ErrNoSelectedSentence},
	{"ACTIVITY_NOT_IN_STATE", http.StatusConflict, domain.ErrActivityNotInState},
	{"NOT_MISTAKE_OWNER", http.StatusForbidden, domain.ErrNotMistakeOwner},
	{"IDEMPOTENCY_KEY_REUSED", http.StatusConflict, domain.ErrIdempotencyKeyReused},
}

// genericCatalog is the implementer-chosen code set for the statuses §1.5
// leaves unnamed (see the table comment in errors.go). The test pins both the
// strings and the statuses so neither can drift silently.
var genericCatalog = []catalogRow{
	{"INVALID_REQUEST", http.StatusBadRequest, domain.ErrInvalidRequest},
	{"UNAUTHORIZED", http.StatusUnauthorized, domain.ErrUnauthorized},
	// §1.5 names a 403 only for mistake ownership, so the repository's other
	// permission sentinels share the generic FORBIDDEN code.
	{"FORBIDDEN", http.StatusForbidden, domain.ErrForbidden},
	{"FORBIDDEN", http.StatusForbidden, domain.ErrNotGuildMember},
	{"FORBIDDEN", http.StatusForbidden, domain.ErrNotGuildMaster},
	{"NOT_FOUND", http.StatusNotFound, domain.ErrNotFound},
	{"CONFLICT", http.StatusConflict, domain.ErrDuplicateEntry},
	{"RELATED_RECORDS_EXIST", http.StatusConflict, domain.ErrHasRelatedRecords},
	{"RATE_LIMITED", http.StatusTooManyRequests, domain.ErrRateLimited},
	{"INTERNAL_ERROR", http.StatusInternalServerError, domain.ErrInternal},
	{"UPSTREAM_UNAVAILABLE", http.StatusBadGateway, domain.ErrUpstreamUnavailable},
}

// documentedCatalog is every code the envelope is allowed to emit.
var documentedCatalog = append(append([]catalogRow{}, specCatalog...), genericCatalog...)

// errorEnvelope decodes the wire shape. The typed ErrorBody means a legacy
// {"error": "...", "details": "..."} body cannot even be decoded.
type errorEnvelope struct {
	Error ErrorBody `json:"error"`
}

// decodeEnvelope asserts the exact envelope shape and returns it. It rejects
// any other top-level shape, any extra key, and a null details.
func decodeEnvelope(t *testing.T, rec *httptest.ResponseRecorder) errorEnvelope {
	t.Helper()
	return decodeEnvelopeBytes(t, rec.Body.Bytes())
}

// decodeEnvelopeBytes is decodeEnvelope for a body captured outside httptest's
// recorder, e.g. from a real HTTP round trip through the reverse proxy.
func decodeEnvelopeBytes(t *testing.T, body []byte) errorEnvelope {
	t.Helper()

	raw := string(body)
	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(raw), &top), "body is not a JSON object: %s", raw)
	require.Len(t, top, 1, "the envelope has exactly one top-level key: %s", raw)

	body, ok := top["error"]
	require.True(t, ok, `the top-level key is "error": %s`, raw)

	var inner map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &inner), `"error" must be an object: %s`, raw)
	keys := make([]string, 0, len(inner))
	for k := range inner {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	require.Equal(t, []string{"code", "details", "message"}, keys,
		`"error" carries exactly code, message and details: %s`, raw)

	var env errorEnvelope
	require.NoError(t, json.Unmarshal([]byte(raw), &env), raw)
	assert.NotEmpty(t, env.Error.Code, "code is never empty")
	assert.NotEmpty(t, env.Error.Message, "message is never empty")
	assert.NotNil(t, env.Error.Details, "details is an object, never null")

	return env
}

// serveError writes err through the central writer and returns the response.
func serveError(t *testing.T, err error) *httptest.ResponseRecorder {
	t.Helper()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/boom", func(c *gin.Context) { writeError(c, err) })

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	return rec
}

// TestWriteErrorCatalog walks every documented sentinel in its bare form, in
// the wrapped form a repository or middleware produces, and with a
// caller-facing message override, asserting status plus error.code each time.
func TestWriteErrorCatalog(t *testing.T) {
	for _, row := range documentedCatalog {
		wrapped := fmt.Errorf("repository: %w", row.sentinel)

		cases := []struct {
			name    string
			err     error
			message string
		}{
			{"bare", row.sentinel, row.sentinel.Error()},
			{"wrapped", wrapped, row.sentinel.Error()},
			{"message override", errorWithMessage(row.sentinel, "resource-specific detail"), "resource-specific detail"},
		}

		for _, tc := range cases {
			t.Run(row.code+"/"+tc.name, func(t *testing.T) {
				rec := serveError(t, tc.err)

				require.Equal(t, row.status, rec.Code, rec.Body.String())

				env := decodeEnvelope(t, rec)
				assert.Equal(t, row.code, env.Error.Code)
				assert.Equal(t, tc.message, env.Error.Message)
				assert.Empty(t, env.Error.Details)
			})
		}
	}
}

// unmappedError is an error type outside the catalog with a method, so the
// writer cannot rely on it being a plain *errors.errorString.
type unmappedError struct{ message string }

func (e *unmappedError) Error() string { return e.message }

// TestWriteErrorUnmappedIsGenericInternal proves that an error outside the
// catalog becomes 500 with the generic message and no internal detail.
func TestWriteErrorUnmappedIsGenericInternal(t *testing.T) {
	const internal = `pq: password authentication failed for user "postgres" (host 203.0.113.9)`
	leaks := []string{"password", "postgres", "203.0.113.9", "load guild"}

	unmapped := []error{
		errors.New(internal),
		fmt.Errorf("load guild: %w", errors.New(internal)),
		&unmappedError{message: internal},
	}

	for _, err := range unmapped {
		rec := serveError(t, err)

		require.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
		env := decodeEnvelope(t, rec)
		assert.Equal(t, "INTERNAL_ERROR", env.Error.Code)
		assert.Equal(t, "internal server error", env.Error.Message)

		body := rec.Body.String()
		for _, leak := range leaks {
			assert.NotContains(t, body, leak, "an unmapped error must not echo its text")
		}
	}
}

// TestErrorCatalogIsTheDocumentedSet ensures the mapping table is exactly the
// §1.5 business catalog plus the documented generic codes: no missing sentinel,
// no duplicate sentinel, no extra business code, and no code reused for two
// different statuses.
func TestErrorCatalogIsTheDocumentedSet(t *testing.T) {
	documented := make(map[error]catalogRow, len(documentedCatalog))
	codeStatus := make(map[string]int, len(documentedCatalog))
	for _, row := range documentedCatalog {
		documented[row.sentinel] = row
		if status, ok := codeStatus[row.code]; ok {
			require.Equal(t, status, row.status, "code %s is documented with two statuses", row.code)
		}
		codeStatus[row.code] = row.status
	}

	mapped := make(map[error]bool, len(errorMappings))
	for _, m := range errorMappings {
		require.False(t, mapped[m.sentinel], "sentinel mapped twice: %v", m.sentinel)
		mapped[m.sentinel] = true

		want, ok := documented[m.sentinel]
		require.True(t, ok, "sentinel %q is not in the documented catalog", m.sentinel)
		assert.Equal(t, want.status, m.status, "status of %s", m.code)
		assert.Equal(t, want.code, m.code, "code of %q", m.sentinel)
	}

	for sentinel, want := range documented {
		assert.True(t, mapped[sentinel], "documented sentinel %q (%s) is missing from the mapping table", sentinel, want.code)
	}

	// Every emitted code must be one of §1.5's eleven or a documented generic
	// code: this is the "no extra business codes" check.
	business := make(map[string]bool, len(specCatalog))
	for _, row := range specCatalog {
		business[row.code] = true
	}
	generic := make(map[string]bool, len(genericCatalog))
	for _, row := range genericCatalog {
		generic[row.code] = true
	}
	for _, m := range errorMappings {
		assert.True(t, business[m.code] || generic[m.code],
			"code %s is neither a §1.5 business code nor a documented generic code", m.code)
	}
}
