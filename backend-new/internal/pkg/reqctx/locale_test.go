// locale_test.go — unit tests for SetLocale / GetLocale.
//
// locale_test.go — SetLocale / GetLocale 的单元测试。
package reqctx

import (
	"context"
	"testing"
)

func TestSetGetLocale_RoundTrip(t *testing.T) {
	ctx := SetLocale(context.Background(), LocaleEn)
	if got := GetLocale(ctx); got != LocaleEn {
		t.Errorf("got %q, want %q", got, LocaleEn)
	}
}

func TestGetLocale_MissingReturnsDefault(t *testing.T) {
	// Unlike userID, locale missing is not a bug — gracefully fall back.
	// 与 userID 不同，locale 缺失不是 bug——优雅降级到默认。
	if got := GetLocale(context.Background()); got != DefaultLocale {
		t.Errorf("got %q, want default %q", got, DefaultLocale)
	}
}

func TestGetLocale_UnsupportedFallsBackToDefault(t *testing.T) {
	// Someone might store an unsupported locale (typo, bad header).
	// GetLocale must silently fall back to DefaultLocale.
	//
	// 有人可能存入不支持的 locale（拼错、坏 header）。GetLocale 必须静默
	// 降级到 DefaultLocale。
	ctx := SetLocale(context.Background(), Locale("fr-FR"))
	if got := GetLocale(ctx); got != DefaultLocale {
		t.Errorf("got %q, want default %q", got, DefaultLocale)
	}
}

func TestGetLocale_PrivateKeyIsolation(t *testing.T) {
	// Private empty-struct key must not collide with external string keys.
	// 私有空结构体 key 不得与外部 string key 冲突。
	ctx := context.WithValue(context.Background(), "locale", "en") //nolint:staticcheck // intentional bad key type
	if got := GetLocale(ctx); got != DefaultLocale {
		t.Errorf("string-keyed value leaked: got %q, want default %q", got, DefaultLocale)
	}
}

func TestLocale_IsSupported(t *testing.T) {
	cases := []struct {
		in   Locale
		want bool
	}{
		{LocaleZhCN, true},
		{LocaleEn, true},
		{Locale(""), false},
		{Locale("fr-FR"), false},
		{Locale("zh"), false}, // must be exact "zh-CN"
	}
	for _, c := range cases {
		if got := c.in.IsSupported(); got != c.want {
			t.Errorf("%q.IsSupported(): got %v, want %v", c.in, got, c.want)
		}
	}
}

func TestDefaultLocale_IsSupported(t *testing.T) {
	// Guard: DefaultLocale must itself be a supported value.
	// 守护：DefaultLocale 自身必须是被支持的 locale。
	if !DefaultLocale.IsSupported() {
		t.Errorf("DefaultLocale %q is not in IsSupported() list", DefaultLocale)
	}
}
