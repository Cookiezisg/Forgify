// aesgcm_test.go — unit tests for the AES-GCM Encryptor implementation.
//
// aesgcm_test.go — AES-GCM Encryptor 实现的单元测试。
package crypto

import (
	"bytes"
	"context"
	"errors"
	"testing"

	domaincrypto "github.com/sunweilin/forgify/backend/internal/domain/crypto"
)

// testKey is a deterministic 32-byte key used across tests.
// 所有测试共用的确定性 32 字节密钥。
var testKey = DeriveKey("test-fingerprint")

func newTestEncryptor(t *testing.T) *AESGCMEncryptor {
	t.Helper()
	e, err := NewAESGCMEncryptor(testKey)
	if err != nil {
		t.Fatalf("construct encryptor: %v", err)
	}
	return e
}

func TestAESGCMEncryptor_SatisfiesInterface(t *testing.T) {
	// Compile-time check: AESGCMEncryptor must implement domain Encryptor.
	// 编译期检查：AESGCMEncryptor 必须实现 domain Encryptor。
	var _ domaincrypto.Encryptor = (*AESGCMEncryptor)(nil)
}

func TestAESGCMEncryptor_RoundTrip(t *testing.T) {
	e := newTestEncryptor(t)
	ctx := context.Background()

	cases := [][]byte{
		[]byte("sk-proj-abcdef1234567890"),
		[]byte(""),
		[]byte("!@#$%^&*()_+"),
		bytes.Repeat([]byte("x"), 10_000), // large payload
	}

	for _, plaintext := range cases {
		t.Run(string(plaintext[:min(len(plaintext), 20)]), func(t *testing.T) {
			ciphertext, err := e.Encrypt(ctx, plaintext)
			if err != nil {
				t.Fatalf("encrypt: %v", err)
			}
			got, err := e.Decrypt(ctx, ciphertext)
			if err != nil {
				t.Fatalf("decrypt: %v", err)
			}
			if !bytes.Equal(got, plaintext) {
				t.Errorf("round-trip mismatch:\n got=%q\nwant=%q", got, plaintext)
			}
		})
	}
}

func TestAESGCMEncryptor_CiphertextHasV1Prefix(t *testing.T) {
	e := newTestEncryptor(t)
	ct, err := e.Encrypt(context.Background(), []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(ct, []byte("v1:")) {
		t.Errorf("ciphertext missing v1: prefix, got %q", ct)
	}
}

func TestAESGCMEncryptor_CiphertextIsNonDeterministic(t *testing.T) {
	// IND-CPA: two encryptions of the same plaintext must produce different
	// ciphertexts (random nonce).
	//
	// IND-CPA 安全：同一明文两次加密必须产生不同密文（随机 nonce）。
	e := newTestEncryptor(t)
	ctx := context.Background()

	ct1, _ := e.Encrypt(ctx, []byte("same-input"))
	ct2, _ := e.Encrypt(ctx, []byte("same-input"))
	if bytes.Equal(ct1, ct2) {
		t.Errorf("two encryptions produced identical ciphertext — nonce reuse bug")
	}
}

func TestAESGCMEncryptor_DifferentKeyFailsDecryption(t *testing.T) {
	// A ciphertext encrypted with one key must NOT decrypt with another.
	// GCM's authentication tag catches this.
	//
	// 用一个密钥加密的密文**不得**被另一个密钥解密。GCM 的认证 tag 会检测到。
	e1, _ := NewAESGCMEncryptor(DeriveKey("machine-A"))
	e2, _ := NewAESGCMEncryptor(DeriveKey("machine-B"))
	ctx := context.Background()

	ct, _ := e1.Encrypt(ctx, []byte("sensitive"))
	_, err := e2.Decrypt(ctx, ct)
	if err == nil {
		t.Errorf("decrypt with wrong key should fail, got nil error")
	}
}

func TestAESGCMEncryptor_UnsupportedVersionPrefix(t *testing.T) {
	// Future v2 ciphertext fed to v1 decryptor → ErrUnsupportedVersion.
	//
	// 把未来的 v2 密文送到 v1 解密器 → ErrUnsupportedVersion。
	e := newTestEncryptor(t)
	_, err := e.Decrypt(context.Background(), []byte("v2:anything-here"))
	if !errors.Is(err, ErrUnsupportedVersion) {
		t.Errorf("want ErrUnsupportedVersion, got %v", err)
	}
}

func TestAESGCMEncryptor_MissingPrefixRejected(t *testing.T) {
	e := newTestEncryptor(t)
	_, err := e.Decrypt(context.Background(), []byte("bare-ciphertext-no-prefix"))
	if !errors.Is(err, ErrUnsupportedVersion) {
		t.Errorf("want ErrUnsupportedVersion, got %v", err)
	}
}

func TestAESGCMEncryptor_ShortCiphertextReturnsError(t *testing.T) {
	// Regression guard for the old code's bug: `if len < nonceSize { return nil, err }`
	// where err was nil, falsely signaling success. We must return a real error.
	//
	// 老代码的 bug 回归守卫：老实现 `if len < nonceSize { return nil, err }`
	// 其中 err 为 nil，伪装成功。我们必须返回真正的错误。
	e := newTestEncryptor(t)
	// "v1:" + base64("shrt") = "v1:c2hydA=="
	short := []byte("v1:c2hydA==")
	plaintext, err := e.Decrypt(context.Background(), short)
	if err == nil {
		t.Errorf("short ciphertext must return error, got plaintext=%q err=nil", plaintext)
	}
	if plaintext != nil {
		t.Errorf("short ciphertext must return nil plaintext, got %q", plaintext)
	}
}

func TestAESGCMEncryptor_TamperedCiphertextRejected(t *testing.T) {
	// Flipping any byte in the ciphertext must cause decrypt to fail (GCM auth).
	//
	// 翻转密文任何一个字节，解密都必须失败（GCM 认证保护）。
	e := newTestEncryptor(t)
	ctx := context.Background()
	ct, _ := e.Encrypt(ctx, []byte("verify me"))
	if len(ct) < 10 {
		t.Skip("ciphertext too short for tampering test")
	}
	// Flip last byte. 翻转最后一字节。
	tampered := append([]byte(nil), ct...)
	tampered[len(tampered)-1] ^= 0xff
	_, err := e.Decrypt(ctx, tampered)
	if err == nil {
		t.Errorf("tampered ciphertext should fail decryption")
	}
}

func TestNewAESGCMEncryptor_RejectsWrongKeySize(t *testing.T) {
	cases := [][]byte{
		nil,
		{},
		bytes.Repeat([]byte{0}, 16), // 16 — AES-128, but we want 256
		bytes.Repeat([]byte{0}, 31),
		bytes.Repeat([]byte{0}, 33),
	}
	for _, key := range cases {
		if _, err := NewAESGCMEncryptor(key); err == nil {
			t.Errorf("key size %d should be rejected", len(key))
		}
	}
}

func TestDeriveKey_DeterministicAndSized(t *testing.T) {
	k1 := DeriveKey("same-input")
	k2 := DeriveKey("same-input")
	if !bytes.Equal(k1, k2) {
		t.Errorf("DeriveKey is not deterministic for same input")
	}
	if len(k1) != 32 {
		t.Errorf("derived key length = %d, want 32", len(k1))
	}
	k3 := DeriveKey("different-input")
	if bytes.Equal(k1, k3) {
		t.Errorf("different inputs produced same key")
	}
}
