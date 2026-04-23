// mask_test.go — unit tests for MaskKey.
//
// mask_test.go — MaskKey 的单元测试。
package tools

import (
	"strings"
	"testing"
)

func TestMaskKey(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// length < 8: fully hidden / 完全隐藏
		{"", "****"},
		{"abc", "****"},
		{"short12", "****"},

		// length 8-20: first 3 + ... + last 4
		{"12345678", "123...5678"},
		{"AKIA1234567890ABCDEF", "AKI...CDEF"},

		// length > 20: first 7 + ... + last 4
		{"sk-proj-abcdefg1234567890xyz", "sk-proj...0xyz"},
		{"sk-ant-api01-xxxxxxxxxxxxxxxxyyyy", "sk-ant-...yyyy"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			if got := MaskKey(c.in); got != c.want {
				t.Errorf("MaskKey(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestMaskKey_NeverLeaksMiddle(t *testing.T) {
	// Safety regression: mask must not include identifiable middle bytes.
	// 安全回归：掩码**不得**包含 key 可识别的中间字节。
	secret := "sk-proj-MIDDLE_SECRET_PART_xyz9"
	masked := MaskKey(secret)
	if strings.Contains(masked, "MIDDLE_SECRET_PART") {
		t.Errorf("mask leaked middle of key: %q", masked)
	}
}
