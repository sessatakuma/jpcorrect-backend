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

func (a *API) MarkAccentHandler(c *gin.Context) {
	a.handlerHelper(c, a.apiToolsURL+"/api/MarkAccent/")
}

func (a *API) MarkFuriganaHandler(c *gin.Context) {
	a.handlerHelper(c, a.apiToolsURL+"/api/MarkFurigana/")
}

func (a *API) UsageQueryHeadWordsHandler(c *gin.Context) {
	a.handlerHelper(c, a.apiToolsURL+"/api/UsageQuery/HeadWords/")
}

func (a *API) UsageQueryURLHandler(c *gin.Context) {
	a.handlerHelper(c, a.apiToolsURL+"/api/UsageQuery/URL/")
}

func (a *API) UsageQueryIDDetailsHandler(c *gin.Context) {
	a.handlerHelper(c, a.apiToolsURL+"/api/UsageQuery/IdDetails/")
}

func (a *API) DictQueryHandler(c *gin.Context) {
	a.handlerHelper(c, a.apiToolsURL+"/api/DictQuery/")
}

func (a *API) SentenceQueryHandler(c *gin.Context) {
	a.handlerHelper(c, a.apiToolsURL+"/api/SentenceQuery/")
}
