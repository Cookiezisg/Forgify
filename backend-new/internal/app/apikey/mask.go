// Package apikey (app layer) owns the Service (CRUD + KeyProvider), the
// HTTP-tester wiring, and any pure helpers (e.g. MaskKey) that only the
// service uses.
//
// All three apikey packages (domain / app / store) declare `package apikey`;
// external callers alias at import (e.g. apikeyapp "…/internal/app/apikey").
//
// Package apikey（app 层）负责 Service（CRUD + KeyProvider）、HTTP-tester
// 的装配、以及只给 Service 用的纯工具函数（如 MaskKey）。
//
// 三个 apikey 包（domain / app / store）都声明 `package apikey`；
// 外部调用方 import 时按角色起别名（如 apikeyapp "…/internal/app/apikey"）。
package apikey

// MaskKey converts a plaintext API key into a display-safe masked form.
//
// Rules:
//   - length <  8  → "****" (fully hidden)
//   - length 8-20  → first 3 + "..." + last 4
//   - length > 20  → first 7 + "..." + last 4
//
// Examples:
//
//	"sk-proj-abcdefg1234567890xyz" → "sk-proj...0xyz"
//	"AKIA1234567890ABCDEF"         → "AKI...CDEF"
//	"short"                        → "****"
//
// MaskKey 把明文 API Key 转成展示安全的掩码。
//
// 规则：
//   - 长度 <  8   → "****"（完全隐藏）
//   - 长度 8-20   → 前 3 + "..." + 后 4
//   - 长度 > 20   → 前 7 + "..." + 后 4
func MaskKey(key string) string {
	n := len(key)
	switch {
	case n < 8:
		return "****"
	case n <= 20:
		return key[:3] + "..." + key[n-4:]
	default:
		return key[:7] + "..." + key[n-4:]
	}
}
