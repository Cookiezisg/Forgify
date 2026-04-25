package reqctx

import "context"

// Locale identifies the user's preferred language for AI-generated content
// (LLM prompts, auto titles, summaries). NOT used for backend error
// messages (those stay English, frontend localizes by error code).
//
// Locale 标识用户偏好的 AI 生成内容语言（LLM 提示、自动标题、摘要）。
// **不用于**后端错误消息（保持英文，前端按 error code 本地化）。
type Locale string

const (
	LocaleZhCN    Locale = "zh-CN"
	LocaleEn      Locale = "en"
	DefaultLocale        = LocaleZhCN
)

// IsSupported reports whether the locale is one this backend handles.
//
// IsSupported 报告该 locale 是否被后端支持。
func (l Locale) IsSupported() bool {
	return l == LocaleZhCN || l == LocaleEn
}

type localeKey struct{}

// SetLocale returns a copy of ctx carrying the given locale.
//
// SetLocale 返回携带给定 locale 的 ctx 拷贝。
func SetLocale(ctx context.Context, l Locale) context.Context {
	return context.WithValue(ctx, localeKey{}, l)
}

// GetLocale retrieves the locale, falling back to DefaultLocale if unset
// or unsupported. Unlike GetUserID it always returns a usable value —
// locale is a preference, not a security identity.
//
// GetLocale 取 locale，缺失或不支持时降级到 DefaultLocale。和 GetUserID
// 不同，它总是返回可用值——locale 是偏好而非安全身份。
func GetLocale(ctx context.Context) Locale {
	if l, ok := ctx.Value(localeKey{}).(Locale); ok && l.IsSupported() {
		return l
	}
	return DefaultLocale
}
