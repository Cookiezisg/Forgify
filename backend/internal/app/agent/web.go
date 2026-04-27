// web.go — Web-access system tools: web_search and fetch_url.
// No API keys required: DuckDuckGo HTML endpoint is open, fetch_url uses
// the public Jina Reader endpoint (r.jina.ai) for clean Markdown output.
//
// web.go — 网络访问 system tools：web_search 和 fetch_url。
// 无需 API Key：DuckDuckGo HTML 端点开放，fetch_url 走公开 Jina Reader（r.jina.ai）。
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// httpClient is shared across web tools. Reusing it allows connection pooling.
//
// httpClient 在 web tool 间共享，复用连接池。
var httpClient = &http.Client{Timeout: 30 * time.Second}

// WebTools returns the two web-access system tools.
//
// WebTools 返回两个网络访问 system tool。
func WebTools() []Tool {
	return []Tool{&WebSearchTool{}, &FetchURLTool{}}
}

// ── web_search ────────────────────────────────────────────────────────────────

// WebSearchTool searches the web using DuckDuckGo's HTML endpoint.
//
// WebSearchTool 使用 DuckDuckGo HTML 端点搜索网页。
type WebSearchTool struct{}

func (t *WebSearchTool) Name() string { return "web_search" }
func (t *WebSearchTool) Description() string {
	return "Search the web for current information using DuckDuckGo. " +
		"Returns a list of results with titles, URLs, and summaries. " +
		"Use this when you need up-to-date information or facts you're not certain about."
}
func (t *WebSearchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query":       {"type": "string", "description": "Search query"},
			"max_results": {"type": "integer", "description": "Maximum results to return (default 5, max 10)"}
		},
		"required": ["query"]
	}`)
}

func (t *WebSearchTool) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("web_search: bad args: %w", err)
	}
	if args.MaxResults <= 0 || args.MaxResults > 10 {
		args.MaxResults = 5
	}

	results, err := duckduckgoSearch(ctx, args.Query, args.MaxResults)
	if err != nil {
		return "", fmt.Errorf("web_search: %w", err)
	}
	b, _ := json.Marshal(results)
	return string(b), nil
}

type searchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// duckduckgoSearch uses DuckDuckGo Lite (POST form) to fetch search results.
// The lite endpoint is stable, requires no API key, and returns plain HTML.
//
// duckduckgoSearch 使用 DuckDuckGo Lite（POST 表单）获取搜索结果。
// Lite 端点稳定、无需 API Key，返回纯 HTML。
func duckduckgoSearch(ctx context.Context, query string, maxResults int) ([]searchResult, error) {
	formBody := strings.NewReader("q=" + url.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://lite.duckduckgo.com/lite/", formBody)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	results, err := parseDDGLiteHTML(resp.Body, maxResults)
	if err != nil {
		return nil, fmt.Errorf("parse results: %w", err)
	}
	return results, nil
}

// parseDDGLiteHTML extracts search results from DuckDuckGo Lite HTML.
// Structure: <a class="result-link"> for title/URL, <td class="result-snippet"> for snippet.
//
// parseDDGLiteHTML 从 DuckDuckGo Lite HTML 提取搜索结果。
// 结构：<a class="result-link"> 含标题/URL，<td class="result-snippet"> 含摘要。
func parseDDGLiteHTML(body io.Reader, maxResults int) ([]searchResult, error) {
	doc, err := html.Parse(body)
	if err != nil {
		return nil, fmt.Errorf("parseDDGLiteHTML: %w", err)
	}

	var results []searchResult
	var pendingURL, pendingTitle string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if len(results) >= maxResults {
			return
		}
		if n.Type == html.ElementNode {
			switch n.Data {
			case "a":
				if attrVal(n, "class") == "result-link" {
					href := attrVal(n, "href")
					if strings.HasPrefix(href, "http") && !strings.Contains(href, "duckduckgo.com") {
						pendingURL = href
						pendingTitle = strings.TrimSpace(textContent(n))
					}
				}
			case "td":
				if attrVal(n, "class") == "result-snippet" && pendingURL != "" {
					snippet := strings.TrimSpace(textContent(n))
					results = append(results, searchResult{
						Title:   pendingTitle,
						URL:     pendingURL,
						Snippet: snippet,
					})
					pendingURL = ""
					pendingTitle = ""
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return results, nil
}

func attrVal(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func textContent(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		sb.WriteString(textContent(c))
	}
	return sb.String()
}

// ── fetch_url ─────────────────────────────────────────────────────────────────

// FetchURLTool fetches a URL and returns its content as clean Markdown
// via the public Jina Reader API (r.jina.ai). No API key required.
//
// FetchURLTool 通过公开 Jina Reader API（r.jina.ai）获取 URL 内容，
// 以干净 Markdown 返回，无需 API Key。
type FetchURLTool struct{}

func (t *FetchURLTool) Name() string { return "fetch_url" }
func (t *FetchURLTool) Description() string {
	return "Fetch the content of any URL and return it as clean readable text (Markdown). " +
		"Useful for reading web pages, documentation, articles, or any public URL. " +
		"Returns the main textual content with images and ads removed."
}
func (t *FetchURLTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url": {"type": "string", "description": "The full URL to fetch (must include http:// or https://)"}
		},
		"required": ["url"]
	}`)
}

func (t *FetchURLTool) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("fetch_url: bad args: %w", err)
	}
	if args.URL == "" {
		return "", fmt.Errorf("fetch_url: url is required")
	}

	// Proxy through Jina Reader to convert HTML → clean Markdown.
	// 走 Jina Reader 把 HTML 转为干净的 Markdown。
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://r.jina.ai/"+args.URL, nil)
	if err != nil {
		return "", fmt.Errorf("fetch_url: build request: %w", err)
	}
	req.Header.Set("Accept", "text/plain")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch_url: request failed: %w", err)
	}
	defer resp.Body.Close()

	// Limit to 50 KB to protect LLM context.
	// 限制 50 KB，保护 LLM context。
	body, err := io.ReadAll(io.LimitReader(resp.Body, 50*1024))
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("fetch_url: read body: %w", err)
	}
	content := strings.TrimSpace(string(body))
	if content == "" {
		return fmt.Sprintf("no content returned from %s", args.URL), nil
	}
	return content, nil
}
