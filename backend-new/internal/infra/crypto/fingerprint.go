// Package crypto implements domain/crypto.Encryptor.
// Current: AES-GCM with master key derived from machine fingerprint.
// Future: envelope encryption backed by cloud KMS (for SaaS).
//
// Package crypto 实现 domain/crypto.Encryptor。
// 当前：AES-GCM，主密钥从机器指纹派生。未来：基于云 KMS 的信封加密（SaaS）。
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
// stable machine identity. Callers MUST refuse to proceed — sharing a
// fallback key across users would be a critical security failure.
//
// ErrNoFingerprint 在 MachineFingerprint 无法确定稳定机器标识时返回。
// 调用方**必须**拒绝继续——共享 fallback 密钥等于严重安全故障。
var ErrNoFingerprint = errors.New("cannot determine machine fingerprint")

// MachineFingerprint returns a stable per-machine identifier suitable for
// deriving encryption keys. NEVER returns a hardcoded fallback.
//
// Platform sources:
//   - darwin:  ioreg IOPlatformSerialNumber
//   - windows: registry MachineGuid
//   - linux:   /etc/machine-id
//
// MachineFingerprint 返回可用于派生加密密钥的稳定机器标识。
// **永不**返回硬编码 fallback。
//
// 平台来源：
//   - darwin:  ioreg IOPlatformSerialNumber
//   - windows: 注册表 MachineGuid
//   - linux:   /etc/machine-id
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
