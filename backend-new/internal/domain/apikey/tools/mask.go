// Package tools holds pure, reusable helpers for the apikey domain —
// things like key masking, validators, formatters. New tools go in their
// own file here; no package-wide shared state.
//
// Package tools 存放 apikey domain 的纯工具函数——如 key 掩码、
// 校验器、格式化器。新工具单独开文件放进来；本包内无共享状态。
package tools

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
