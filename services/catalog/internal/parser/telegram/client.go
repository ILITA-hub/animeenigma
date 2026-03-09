package telegram

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const maxBodySize = 2 * 1024 * 1024 // 2MB

const (
	baseURL  = "https://t.me/s"
	maxPosts = 20
)

// NewsItem represents a single Telegram channel post
type NewsItem struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	ImageURL string `json:"image_url,omitempty"`
	Date     string `json:"date"`
	Link     string `json:"link"`
	Views    string `json:"views"`
}

// Client is the Telegram channel scraper client
type Client struct {
	httpClient  *http.Client
	newsChannel string
}

// NewClient creates a new Telegram scraper client
func NewClient(newsChannel string) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		newsChannel: newsChannel,
	}
}

// bgImageRegexp extracts URL from background-image:url('...')
var bgImageRegexp = regexp.MustCompile(`background-image:\s*url\(['"]?([^'")\s]+)['"]?\)`)

// FetchNews scrapes the public Telegram channel web preview and returns parsed news items
func (c *Client) FetchNews(ctx context.Context) ([]NewsItem, error) {
	url := fmt.Sprintf("%s/%s", baseURL, c.newsChannel)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("telegram: failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; AnimeEnigma/1.0)")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("telegram: failed to fetch channel: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("telegram: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	doc, err := goquery.NewDocumentFromReader(io.LimitReader(resp.Body, maxBodySize))
	if err != nil {
		return nil, fmt.Errorf("telegram: failed to parse HTML: %w", err)
	}

	var items []NewsItem

	doc.Find(".tgme_widget_message").Each(func(i int, s *goquery.Selection) {
		item := NewsItem{}

		// Extract message ID and link from data-post attribute
		if dataPost, exists := s.Attr("data-post"); exists {
			// data-post is like "channelname/123"
			parts := strings.SplitN(dataPost, "/", 2)
			if len(parts) == 2 {
				item.ID = parts[1]
				item.Link = fmt.Sprintf("https://t.me/%s/%s", c.newsChannel, parts[1])
			}
		}

		// Extract text content
		textEl := s.Find(".tgme_widget_message_text")
		if textEl.Length() > 0 {
			// Get HTML, convert <br> to newlines, then strip remaining tags
			html, _ := textEl.Html()
			// Replace <br> and <br/> with newlines
			html = strings.ReplaceAll(html, "<br>", "\n")
			html = strings.ReplaceAll(html, "<br/>", "\n")
			html = strings.ReplaceAll(html, "<br />", "\n")
			// Strip remaining HTML tags
			item.Text = stripHTMLTags(html)
			item.Text = strings.TrimSpace(item.Text)
		}

		// Extract image URL from background-image style
		photoWrap := s.Find(".tgme_widget_message_photo_wrap")
		if photoWrap.Length() > 0 {
			if style, exists := photoWrap.Attr("style"); exists {
				matches := bgImageRegexp.FindStringSubmatch(style)
				if len(matches) > 1 {
					item.ImageURL = matches[1]
				}
			}
		}

		// Extract date from datetime attribute
		timeEl := s.Find(".tgme_widget_message_date time")
		if timeEl.Length() > 0 {
			if datetime, exists := timeEl.Attr("datetime"); exists {
				item.Date = datetime
			}
		}

		// Extract view count
		viewsEl := s.Find(".tgme_widget_message_views")
		if viewsEl.Length() > 0 {
			item.Views = strings.TrimSpace(viewsEl.Text())
		}

		// Only add items that have at least an ID and some content
		if item.ID != "" && (item.Text != "" || item.ImageURL != "") {
			items = append(items, item)
		}
	})

	// Limit to last N posts (they appear in chronological order, most recent last)
	if len(items) > maxPosts {
		items = items[len(items)-maxPosts:]
	}

	// Reverse so newest posts come first
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}

	return items, nil
}

// stripHTMLTags removes HTML tags from a string
func stripHTMLTags(s string) string {
	var result strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			result.WriteRune(r)
		}
	}
	return html.UnescapeString(result.String())
}
