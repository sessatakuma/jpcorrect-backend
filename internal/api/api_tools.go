package api

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/gin-gonic/gin"
)

func (a *API) handlerHelper(c *gin.Context, target string) {
	remote, err := url.Parse(target)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Invalid target URL"})
		return
	}

	proxy := &httputil.ReverseProxy{
		Transport: a.proxyTransport,

		Director: func(req *http.Request) {
			req.URL = remote
			req.Host = remote.Host
			if a.apiToolsKey != "" {
				req.Header.Set("X-API-KEY", a.apiToolsKey)
			}
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
// @Description Analyze Japanese text and return accent (pitch) patterns for each word. The accent_marking_type values: 0=no accent, 1=heiban (flat), 2=fall down. Supports kanji-kana mixed input. Requires text input.
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

// @Summary Mark furigana
// @Description Annotate Japanese text with furigana (reading aid) readings. Breaks input into words, providing furigana and optional sub-word decomposition for mixed kanji-kana words. Requires text input.
// @Tags api-tools
// @Accept json
// @Produce json
// @Param body body MarkFuriganaRequest true "Japanese text to annotate with furigana"
// @Success 200 {object} MarkFuriganaResponse
// @Failure 502 {object} map[string]string
// @Security ApiKeyAuth
// @Router /v1/mark-furigana [post]
func (a *API) MarkFuriganaHandler(c *gin.Context) {
	a.handlerHelper(c, a.apiToolsURL+"/api/MarkFurigana/")
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