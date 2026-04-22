// Package crypto provides concrete implementations of domain/crypto.
//
// Current implementation: AES-GCM with a master key derived from the host's
// machine fingerprint. Future implementation (not in this file): envelope
// encryption backed by a cloud KMS, for the SaaS variant of Forgify.
//
// Package crypto 提供 domain/crypto 的具体实现。
//
// 当前实现：AES-GCM，主密钥从主机的机器指纹派生。未来实现（不在本文件）：
// 基于云 KMS 的信封加密，用于 Forgify 的 SaaS 版本。
package crypto

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// ErrNoFingerprint is returned when MachineFingerprint cannot determine a
// stable machine identity. Callers MUST refuse to proceed rather than use
// a weak fallback — sharing a single encryption key across all users would
// be a catastrophic security failure (one DB leak exposes everything).
//
// ErrNoFingerprint 在 MachineFingerprint 无法确定稳定机器标识时返回。
// 调用方**必须**拒绝继续，而不是 fallback 到弱默认值——让所有用户共享
// 一个加密密钥是灾难级安全故障（单次 DB 泄漏波及所有数据）。
var ErrNoFingerprint = errors.New("cannot determine machine fingerprint")

// MachineFingerprint returns a stable per-machine identifier suitable for
// deriving encryption keys. It NEVER returns a hardcoded fallback — the
// old implementation's "forgify-fallback-key" was a critical vulnerability.
//
// Platform support:
//   - darwin:  IOPlatformSerialNumber via `ioreg`
//   - windows: MachineGuid via registry
//   - linux:   /etc/machine-id
//
// If the platform probe fails, ErrNoFingerprint is returned — the caller
// should abort startup.
//
// MachineFingerprint 返回可用于派生加密密钥的稳定机器标识。它**永远不会**
// 返回硬编码的 fallback——老实现里的 "forgify-fallback-key" 是一个严重
// 安全漏洞。
//
// 平台支持：
//   - darwin:  通过 ioreg 拿 IOPlatformSerialNumber
//   - windows: 读注册表 MachineGuid
//   - linux:   读 /etc/machine-id
//
// 若平台探测失败，返回 ErrNoFingerprint——调用方应终止启动。
func MachineFingerprint() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return fingerprintDarwin()
	case "windows":
		return fingerprintWindows()
	default:
		return fingerprintLinux()
	}
}

func fingerprintDarwin() (string, error) {
	out, err := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
	if err != nil {
		return "", fmt.Errorf("%w: ioreg failed: %v", ErrNoFingerprint, err)
	}
	for line := range strings.SplitSeq(string(out), "\n") {
		if !strings.Contains(line, "IOPlatformSerialNumber") {
			continue
		}
		parts := strings.Split(line, "\"")
		if len(parts) >= 4 && parts[3] != "" {
			return parts[3], nil
		}
	}
	return "", fmt.Errorf("%w: IOPlatformSerialNumber not found in ioreg output", ErrNoFingerprint)
}

func fingerprintWindows() (string, error) {
	out, err := exec.Command("reg", "query",
		`HKLM\SOFTWARE\Microsoft\Cryptography`, "/v", "MachineGuid").Output()
	if err != nil {
		return "", fmt.Errorf("%w: reg query failed: %v", ErrNoFingerprint, err)
	}
	parts := strings.Fields(string(out))
	if len(parts) == 0 {
		return "", fmt.Errorf("%w: empty reg query output", ErrNoFingerprint)
	}
	guid := parts[len(parts)-1]
	if guid == "" || guid == "MachineGuid" {
		return "", fmt.Errorf("%w: MachineGuid value missing", ErrNoFingerprint)
	}
	return guid, nil
}

func fingerprintLinux() (string, error) {
	data, err := os.ReadFile("/etc/machine-id")
	if err != nil {
		return "", fmt.Errorf("%w: read /etc/machine-id: %v", ErrNoFingerprint, err)
	}
	id := strings.TrimSpace(string(data))
	if id == "" {
		return "", fmt.Errorf("%w: /etc/machine-id is empty", ErrNoFingerprint)
	}
	return id, nil
}
