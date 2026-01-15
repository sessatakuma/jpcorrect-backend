package api

import (
	"bytes"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

func (a *API) handlerHelper(c *gin.Context, targetURL string) {
	// Read the raw body
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	// Create a new request to the external API
	req, err := http.NewRequest("POST", targetURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
		return
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	resp, err := a.httpClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to contact external API"})
		return
	}
	defer resp.Body.Close()

	// Read the response from the external API
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read response from external API"})
		return
	}

	// Return the external API's response
	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), respBody)
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
