package types

import "errors"

// Sentinel errors. Mapped to HTTP responses by
// transport/httpapi/response/errmap.go.
//
// Sentinel 错误。由 transport/httpapi/response/errmap.go 映射到 HTTP 响应。
var (
	// ErrNotFound: lookup by id did not match a live record.
	// ErrNotFound：按 id 查询未命中活跃记录。
	ErrNotFound = errors.New("apikey: not found")

	// ErrNotFoundForProvider: current user has no live Key for the given provider.
	// ErrNotFoundForProvider：当前用户在给定 provider 下没有活跃 Key。
	ErrNotFoundForProvider = errors.New("apikey: no key for provider")

	// ErrInvalidProvider: provider name not in the supported whitelist.
	// ErrInvalidProvider：provider 名称不在支持的白名单内。
	ErrInvalidProvider = errors.New("apikey: invalid provider")

	// ErrBaseURLRequired: provider requires a base_url (ollama / custom) but none given.
	// ErrBaseURLRequired：provider 要求 base_url（ollama / custom），但未提供。
	ErrBaseURLRequired = errors.New("apikey: base_url required for this provider")

	// ErrAPIFormatRequired: custom provider needs an api_format.
	// ErrAPIFormatRequired：custom provider 必须指定 api_format。
	ErrAPIFormatRequired = errors.New("apikey: api_format required for custom provider")

	// ErrKeyRequired: create request missing the key value.
	// ErrKeyRequired：创建请求缺少 key 值。
	ErrKeyRequired = errors.New("apikey: key value is required")

	// ErrTestFailed: connectivity test failed (request reached provider, provider rejected).
	// ErrTestFailed：连通性测试失败（请求已达 provider，但被拒或出错）。
	ErrTestFailed = errors.New("apikey: connectivity test failed")

	// ErrInvalid: provider returned 401/403 at actual LLM call time.
	// ErrInvalid：provider 在真实 LLM 调用时返回 401/403。
	ErrInvalid = errors.New("apikey: key rejected by provider")
)
