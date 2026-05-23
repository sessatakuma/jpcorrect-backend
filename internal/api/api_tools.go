package api

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
)

func (a *API) handlerHelper(c *gin.Context, target string) {
	a.proxyTo(c, target, 0)
}

// streamingHelper proxies an upstream NDJSON / streaming response. FlushInterval=-1
// makes the reverse proxy flush after every write so clients see each chunk
// as soon as api-tools emits it.
func (a *API) streamingHelper(c *gin.Context, target string) {
	a.proxyTo(c, target, -1)
}

func (a *API) proxyTo(c *gin.Context, target string, flushInterval time.Duration) {
	remote, err := url.Parse(target)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid target URL"})
		return
	}

	proxy := &httputil.ReverseProxy{
		Transport:     a.proxyTransport,
		FlushInterval: flushInterval,

		Director: func(req *http.Request) {
			req.URL = remote
			req.Host = remote.Host
		},

		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error": "Failed to contact external API"}`))
		},
	}

	proxy.ServeHTTP(c.Writer, c.Request)
}

// @Summary Mark Japanese accent
// @Description Analyze Japanese text and return per-mora pitch (accent) patterns for each word. `accent_marking_type` values: 0=low/unknown, 1=heiban (high plateau), 2=fall kernel. Optional flags control whether English-letter and katakana tokens carry furigana, and `script` rewrites every furigana field to hiragana, katakana, or romaji.
// @Tags api-tools
// @Accept json
// @Produce json
// @Param body body MarkAccentRequest true "Japanese text to analyze for accent patterns"
// @Success 200 {object} MarkAccentResponse
// @Failure 502 {object} map[string]string
// @Security ApiKeyAuth
// @Router /v1/mark-accent [post]
func (a *API) MarkAccentHandler(c *gin.Context) {
	a.handlerHelper(c, a.apiToolsURL+"/api/MarkAccent/")
}

// @Summary Mark Japanese accent (streaming NDJSON)
// @Description Same input and per-chunk output as /v1/mark-accent, but streamed as NDJSON: one JSON object per line, emitted as soon as each input chunk finishes. Each line carries `{"chunk": <line_idx>, "subchunk": <sub_idx>, ...AccentResponse}` so clients can interleave UI rendering with later chunks still in flight.
// @Tags api-tools
// @Accept json
// @Produce application/x-ndjson
// @Param body body MarkAccentRequest true "Japanese text to analyze for accent patterns"
// @Success 200 {string} string "NDJSON stream of per-chunk AccentResponse objects"
// @Failure 502 {object} map[string]string
// @Security ApiKeyAuth
// @Router /v1/mark-accent/stream [post]
func (a *API) MarkAccentStreamHandler(c *gin.Context) {
	a.streamingHelper(c, a.apiToolsURL+"/api/MarkAccent/stream/")
}

// @Summary Query usage headwords
// @Description Search for headwords in Japanese language corpora. Supports lookup by kanji, kana, or romaji. Returns matching headword entries with readings and frequency data. Specify site as NLB (NINJAL LRP BCCWJ) or NLT (Tsukuba Web Corpus).
// @Tags api-tools
// @Accept json
// @Produce json
// @Param body body UsageQueryHeadWordsRequest true "Word to search (kanji, kana, or romaji) and corpus site"
// @Success 200 {object} UsageQueryHeadWordsResponse
// @Failure 502 {object} map[string]string
// @Security ApiKeyAuth
// @Router /v1/usage-query/headwords [post]
func (a *API) UsageQueryHeadWordsHandler(c *gin.Context) {
	a.handlerHelper(c, a.apiToolsURL+"/api/UsageQuery/HeadWords/")
}

// @Summary Query usage by URL
// @Description Given a word, return URLs to the corresponding headword detail pages in the selected corpus (NLB or NLT). Uses the same request format as the headwords endpoint.
// @Tags api-tools
// @Accept json
// @Produce json
// @Param body body UsageQueryHeadWordsRequest true "Word to search and corpus site"
// @Success 200 {object} UsageQueryURLResponse
// @Failure 502 {object} map[string]string
// @Security ApiKeyAuth
// @Router /v1/usage-query/url [post]
func (a *API) UsageQueryURLHandler(c *gin.Context) {
	a.handlerHelper(c, a.apiToolsURL+"/api/UsageQuery/URL/")
}

// @Summary Query usage by ID details
// @Description Retrieve detailed usage information for a specific headword by its ID. Returns base form, subcorpus distribution, conjugation patterns (shojikei/katuyokei), and collocation data. The headword_id must be obtained from the headwords endpoint first.
// @Tags api-tools
// @Accept json
// @Produce json
// @Param body body UsageQueryIDDetailsRequest true "Headword ID and corpus site"
// @Success 200 {object} UsageQueryIDDetailsResponse
// @Failure 502 {object} map[string]string
// @Security ApiKeyAuth
// @Router /v1/usage-query/id-details [post]
func (a *API) UsageQueryIDDetailsHandler(c *gin.Context) {
	a.handlerHelper(c, a.apiToolsURL+"/api/UsageQuery/IdDetails/")
}

// @Summary Dictionary query
// @Description Look up words in JMdict (Japanese-Multilingual Dictionary). Returns kanji forms, furigana readings, parts of speech, and English definitions. Supports searching by kanji, kana, or romaji.
// @Tags api-tools
// @Accept json
// @Produce json
// @Param body body DictQueryRequest true "Word to look up in the dictionary"
// @Success 200 {object} DictQueryResponse
// @Failure 502 {object} map[string]string
// @Security ApiKeyAuth
// @Router /v1/dict-query [post]
func (a *API) DictQueryHandler(c *gin.Context) {
	a.handlerHelper(c, a.apiToolsURL+"/api/DictQuery/")
}

// @Summary Sentence query
// @Description Find example sentences containing a given word from JMdict. Requires both the word and its JMdict entry ID (obtained from the dict-query endpoint). Returns bilingual Japanese-English sentence pairs.
// @Tags api-tools
// @Accept json
// @Produce json
// @Param body body SentenceQueryRequest true "Word and its JMdict entry ID"
// @Success 200 {object} SentenceQueryResponse
// @Failure 502 {object} map[string]string
// @Security ApiKeyAuth
// @Router /v1/sentence-query [post]
func (a *API) SentenceQueryHandler(c *gin.Context) {
	a.handlerHelper(c, a.apiToolsURL+"/api/SentenceQuery/")
}
