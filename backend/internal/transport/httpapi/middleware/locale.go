package middleware

import (
	"net/http"
	"strings"

	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// InjectLocale parses Accept-Language and stamps ctx with a supported
// Locale. Unsupported / missing → reqctxpkg.DefaultLocale.
//
// InjectLocale 解析 Accept-Language 并塞入支持的 Locale。
// 不支持或缺失则降级到 reqctxpkg.DefaultLocale。
func InjectLocale(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loc := parseAcceptLanguage(r.Header.Get("Accept-Language"))
		ctx := reqctxpkg.SetLocale(r.Context(), loc)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// parseAcceptLanguage does a simplified BCP47 prefix match: "en*" → en,
// everything else → zh-CN. Upgrade to x/text/language if we add more
// locales.
//
// parseAcceptLanguage 做简化的 BCP47 前缀匹配："en*" → en，其他 → zh-CN。
// 未来加更多 locale 时升级到 x/text/language。
func parseAcceptLanguage(header string) reqctxpkg.Locale {
	header = strings.ToLower(strings.TrimSpace(header))
	if strings.HasPrefix(header, "en") {
		return reqctxpkg.LocaleEn
	}
	return reqctxpkg.LocaleZhCN
}
