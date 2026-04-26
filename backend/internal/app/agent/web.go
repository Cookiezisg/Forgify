// web.go — Web-access system tools: web_search and fetch_url.
// No API keys required: DuckDuckGo is open, fetch_url uses the public
// Jina Reader endpoint (r.jina.ai) which converts any URL to clean Markdown.
//
// web.go — 网络访问 system tools：web_search 和 fetch_url。
// 无需 API Key：DuckDuckGo 开放使用，fetch_url 走公开的 Jina Reader
// 端点（r.jina.ai）将任意 URL 转为干净的 Markdown。
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	duckduckgo "github.com/cloudwego/eino-ext/components/tool/duckduckgo/v2"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// WebTools returns the two web-access system tools.
// Assembly into the full tool list happens in app/chat (main.go wiring).
//
// WebTools 返回两个网络访问 system tool。
// 完整工具列表的组装在 app/chat（main.go 装配）完成。
func WebTools(ctx context.Context) ([]tool.BaseTool, error) {
	search, err := duckduckgo.NewTextSearchTool(ctx, &duckduckgo.Config{
		ToolName:   "web_search",
		ToolDesc:   "Search the web for current information using DuckDuckGo. Returns a list of results with titles, URLs, and summaries. Use this when you need up-to-date information or facts you're not certain about.",
		MaxResults: 5,
	})
	if err != nil {
		return nil, fmt.Errorf("WebTools: duckduckgo: %w", err)
	}
	return []tool.BaseTool{search, &FetchURLTool{}}, nil
}

// ── fetch_url ─────────────────────────────────────────────────────────────────

// FetchURLTool fetches a URL and returns its content as clean Markdown
// by proxying through the public Jina Reader API (r.jina.ai).
// No API key required for standard usage.
//
// FetchURLTool 通过公开的 Jina Reader API（r.jina.ai）获取 URL 内容，
// 以干净的 Markdown 返回。标准用量无需 API Key。
type FetchURLTool struct{}

func (t *FetchURLTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "fetch_url",
		Desc: "Fetch the content of any URL and return it as clean readable text (Markdown). " +
			"Useful for reading web pages, documentation, articles, or any public URL. " +
			"Returns the main textual content with images and ads removed.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"url": {
				Type:     schema.String,
				Required: true,
				Desc:     "The full URL to fetch (must include http:// or https://)",
			},
		}),
	}, nil
}

func (t *FetchURLTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("fetch_url: bad args: %w", err)
	}
	if args.URL == "" {
		return "", fmt.Errorf("fetch_url: url is required")
	}

	// Prefix with Jina Reader to convert HTML → clean Markdown.
	// 加 Jina Reader 前缀，将 HTML 转为干净的 Markdown。
	jinaURL := "https://r.jina.ai/" + args.URL

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jinaURL, nil)
	if err != nil {
		return "", fmt.Errorf("fetch_url: build request: %w", err)
	}
	req.Header.Set("Accept", "text/plain")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch_url: request failed: %w", err)
	}
	defer resp.Body.Close()

	// Limit response to 50 KB to avoid flooding the LLM context.
	// 限制响应最多 50 KB，避免撑爆 LLM context。
	body, err := io.ReadAll(io.LimitReader(resp.Body, 50*1024))
	if err != nil {
		return "", fmt.Errorf("fetch_url: read body: %w", err)
	}
	content := strings.TrimSpace(string(body))
	if content == "" {
		return fmt.Sprintf("fetch_url: no content returned from %s", args.URL), nil
	}
	return content, nil
}
