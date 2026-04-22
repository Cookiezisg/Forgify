// fingerprint_test.go — tests for MachineFingerprint.
//
// We can't mock the OS, so these tests depend on the host: as long as the
// test runs on a real darwin / windows / linux machine with a machine ID,
// it should succeed. On odd sandboxed environments it may be skipped.
//
// fingerprint_test.go — MachineFingerprint 的测试。
//
// 无法 mock 操作系统，所以测试依赖宿主机：只要跑在正常的 darwin / windows /
// linux 机器上且能拿到机器 ID，就会通过。在奇怪的沙箱环境下测试可能跳过。
package crypto

import (
	"errors"
	"testing"
)

func TestMachineFingerprint_ReturnsNonEmpty(t *testing.T) {
	fp, err := MachineFingerprint()
	if errors.Is(err, ErrNoFingerprint) {
		t.Skip("sandbox denies machine-ID probes, skipping:", err)
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fp == "" {
		t.Errorf("fingerprint is empty string")
	}
}

func TestMachineFingerprint_Deterministic(t *testing.T) {
	a, err := MachineFingerprint()
	if errors.Is(err, ErrNoFingerprint) {
		t.Skip("sandbox denies machine-ID probes, skipping")
	}
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	b, err := MachineFingerprint()
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if a != b {
		t.Errorf("fingerprint is not deterministic: %q vs %q", a, b)
	}
}

func TestMachineFingerprint_NoHardcodedFallback(t *testing.T) {
	// Regression guard: the old implementation returned "forgify-fallback-key"
	// on any failure, which was a critical vulnerability. This test ensures
	// we never, ever, accidentally reintroduce that or any similar constant.
	//
	// 回归守卫：老实现在任何失败时返回 "forgify-fallback-key"，是严重漏洞。
	// 本测试确保我们**永不**再意外引入该常量或任何类似的硬编码值。
	fp, _ := MachineFingerprint()
	forbidden := []string{"forgify-fallback-key", "fallback", "default", "unknown"}
	for _, bad := range forbidden {
		if fp == bad {
			t.Errorf("fingerprint returned forbidden fallback %q", bad)
		}
	}
}
