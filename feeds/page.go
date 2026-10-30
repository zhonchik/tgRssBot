package feeds

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	readability "codeberg.org/readeck/go-readability/v2"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/html/charset"
	"tgRssBot/types"
)

const maxArticleBytes = 5 << 20

var articleClient = &http.Client{Timeout: 15 * time.Second}

// EnrichArticle runs only for undelivered news. Feed content remains available
// when a publisher blocks requests or its page has no extractable article.
func EnrichArticle(message types.Message) types.Message {
	enriched, err := fetchArticle(articleClient, message)
	if err != nil {
		log.WithError(err).Debug("Using feed content for article")
		return message
	}
	return enriched
}

func fetchArticle(client *http.Client, message types.Message) (types.Message, error) {
	u, err := url.Parse(message.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return message, fmt.Errorf("invalid article URL")
	}
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return message, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; RSSArticleReader/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	response, err := client.Do(req)
	if err != nil {
		return message, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return message, fmt.Errorf("article HTTP status %d", response.StatusCode)
	}
	contentType := response.Header.Get("Content-Type")
	if contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil || (mediaType != "text/html" && mediaType != "application/xhtml+xml") {
			return message, fmt.Errorf("article is not HTML")
		}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxArticleBytes+1))
	if err != nil {
		return message, err
	}
	if len(body) > maxArticleBytes {
		return message, fmt.Errorf("article exceeds size limit")
	}
	decoded, err := charset.NewReader(bytes.NewReader(body), contentType)
	if err != nil {
		return message, err
	}
	article, err := readability.FromReader(decoded, response.Request.URL)
	if err != nil {
		return message, err
	}
	var rendered strings.Builder
	if err := article.RenderHTML(&rendered); err != nil {
		return message, err
	}
	text, inlineImage := articleContent(rendered.String())
	if strings.TrimSpace(text) == "" {
		return message, fmt.Errorf("empty article body")
	}
	message.Text = text
	if strings.TrimSpace(message.Title) == "" {
		message.Title = article.Title()
	}
	if message.ImageURL == "" {
		for _, image := range []string{article.ImageURL(), inlineImage} {
			if message.ImageURL = resolveImage(response.Request.URL.String(), image); message.ImageURL != "" {
				break
			}
		}
	}
	return message, nil
}
