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
		Transport: a.proxyTransport, // Use the custom transport for connection pooling

		Director: func(req *http.Request) {
			req.URL = remote // ! It replaces query parameters too
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
// @Description Proxy to external API tools service for marking accent
// @Tags api-tools
// @Accept json
// @Produce json
// @Param body body object true "Request body (forwarded to API tools)"
// @Success 200 {object} object
// @Failure 502 {object} map[string]string
// @Router /v1/mark-accent [post]
func (a *API) MarkAccentHandler(c *gin.Context) {
	a.handlerHelper(c, a.apiToolsURL+"/api/MarkAccent/")
}

// @Summary Mark furigana
// @Description Proxy to external API tools service for marking furigana
// @Tags api-tools
// @Accept json
// @Produce json
// @Param body body object true "Request body (forwarded to API tools)"
// @Success 200 {object} object
// @Failure 502 {object} map[string]string
// @Router /v1/mark-furigana [post]
func (a *API) MarkFuriganaHandler(c *gin.Context) {
	a.handlerHelper(c, a.apiToolsURL+"/api/MarkFurigana/")
}

// @Summary Query usage headwords
// @Description Proxy to external API tools service for usage query headwords
// @Tags api-tools
// @Accept json
// @Produce json
// @Param body body object true "Request body (forwarded to API tools)"
// @Success 200 {object} object
// @Failure 502 {object} map[string]string
// @Router /v1/usage-query/headwords [post]
func (a *API) UsageQueryHeadWordsHandler(c *gin.Context) {
	a.handlerHelper(c, a.apiToolsURL+"/api/UsageQuery/HeadWords/")
}

// @Summary Query usage by URL
// @Description Proxy to external API tools service for usage query by URL
// @Tags api-tools
// @Accept json
// @Produce json
// @Param body body object true "Request body (forwarded to API tools)"
// @Success 200 {object} object
// @Failure 502 {object} map[string]string
// @Router /v1/usage-query/url [post]
func (a *API) UsageQueryURLHandler(c *gin.Context) {
	a.handlerHelper(c, a.apiToolsURL+"/api/UsageQuery/URL/")
}

// @Summary Query usage by ID details
// @Description Proxy to external API tools service for usage query by ID details
// @Tags api-tools
// @Accept json
// @Produce json
// @Param body body object true "Request body (forwarded to API tools)"
// @Success 200 {object} object
// @Failure 502 {object} map[string]string
// @Router /v1/usage-query/id-details [post]
func (a *API) UsageQueryIDDetailsHandler(c *gin.Context) {
	a.handlerHelper(c, a.apiToolsURL+"/api/UsageQuery/IdDetails/")
}

// @Summary Dictionary query
// @Description Proxy to external API tools service for dictionary queries
// @Tags api-tools
// @Accept json
// @Produce json
// @Param body body object true "Request body (forwarded to API tools)"
// @Success 200 {object} object
// @Failure 502 {object} map[string]string
// @Router /v1/dict-query [post]
func (a *API) DictQueryHandler(c *gin.Context) {
	a.handlerHelper(c, a.apiToolsURL+"/api/DictQuery/")
}

// @Summary Sentence query
// @Description Proxy to external API tools service for sentence queries
// @Tags api-tools
// @Accept json
// @Produce json
// @Param body body object true "Request body (forwarded to API tools)"
// @Success 200 {object} object
// @Failure 502 {object} map[string]string
// @Router /v1/sentence-query [post]
func (a *API) SentenceQueryHandler(c *gin.Context) {
	a.handlerHelper(c, a.apiToolsURL+"/api/SentenceQuery/")
}
