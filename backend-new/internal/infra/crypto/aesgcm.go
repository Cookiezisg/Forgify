package crypto

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// v1Prefix tags ciphertexts produced by AESGCMEncryptor. Future algorithms
// (KMS envelope) will use "v2:", letting both coexist in the database.
//
// v1Prefix 标识 AESGCMEncryptor 产生的密文。未来算法（KMS 信封）用 "v2:"，
// 使新旧密文在数据库中共存。
const v1Prefix = "v1:"

// AESGCMEncryptor implements domain/crypto.Encryptor using AES-256-GCM
// with a fixed 32-byte master key. On-wire format:
//
//	"v1:" + base64(nonce || ciphertext || gcm_tag)
//
// AESGCMEncryptor 用 AES-256-GCM + 32 字节主密钥实现 domain/crypto.Encryptor。
// 线上格式：
//
//	"v1:" + base64(nonce || 密文 || gcm_tag)
type AESGCMEncryptor struct {
	gcm cipher.AEAD
}

// NewAESGCMEncryptor constructs an encryptor from a 32-byte master key.
// Use DeriveKey to build the key from a machine fingerprint.
//
// NewAESGCMEncryptor 用 32 字节主密钥构造 encryptor。
// 用 DeriveKey 从机器指纹派生密钥。
func NewAESGCMEncryptor(masterKey []byte) (*AESGCMEncryptor, error) {
	if len(masterKey) != 32 {
		return nil, fmt.Errorf("aesgcm: master key must be 32 bytes, got %d", len(masterKey))
	}
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, fmt.Errorf("aesgcm: new cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("aesgcm: new GCM: %w", err)
	}
	return &AESGCMEncryptor{gcm: gcm}, nil
}

// DeriveKey stretches a fingerprint into a 32-byte AES key using SHA-256
// with a static salt. Changing the salt invalidates all v1 ciphertexts.
//
// DeriveKey 用 SHA-256 + 静态 salt 把指纹拉伸为 32 字节 AES 密钥。
// 修改 salt 会让所有 v1 密文失效。
func DeriveKey(fingerprint string) []byte {
	const salt = "forgify:aesgcm:v1:1ZOI95qH2X" // do not change / 勿改
	h := sha256.Sum256([]byte(salt + "|" + fingerprint))
	return h[:]
}

// Encrypt generates a fresh random nonce per call (IND-CPA security).
//
// Encrypt 每次调用生成全新随机 nonce（IND-CPA 安全性）。
func (e *AESGCMEncryptor) Encrypt(_ context.Context, plaintext []byte) ([]byte, error) {
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("aesgcm: generate nonce: %w", err)
	}
	sealed := e.gcm.Seal(nonce, nonce, plaintext, nil)
	encoded := base64.StdEncoding.EncodeToString(sealed)
	return []byte(v1Prefix + encoded), nil
}

// Decrypt rejects any input lacking the v1 prefix with ErrUnsupportedVersion.
//
// Decrypt 拒绝无 v1 前缀的输入，返回 ErrUnsupportedVersion。
func (e *AESGCMEncryptor) Decrypt(_ context.Context, ciphertext []byte) ([]byte, error) {
	if !bytes.HasPrefix(ciphertext, []byte(v1Prefix)) {
		return nil, ErrUnsupportedVersion
	}
	encoded := ciphertext[len(v1Prefix):]
	sealed, err := base64.StdEncoding.DecodeString(string(encoded))
	if err != nil {
		return nil, fmt.Errorf("aesgcm: base64 decode: %w", err)
	}
	nonceSize := e.gcm.NonceSize()
	if len(sealed) < nonceSize {
		return nil, fmt.Errorf("aesgcm: ciphertext too short (%d < %d)", len(sealed), nonceSize)
	}
	plaintext, err := e.gcm.Open(nil, sealed[:nonceSize], sealed[nonceSize:], nil)
	if err != nil {
		// Includes auth failures: wrong key, tampered data, etc.
		// 含认证失败：错误密钥、被篡改的数据等。
		return nil, fmt.Errorf("aesgcm: open: %w", err)
	}
	return plaintext, nil
}

// ErrUnsupportedVersion is returned when Decrypt sees a ciphertext it
// wasn't designed to handle (e.g. v2 fed to a v1 decryptor).
//
// ErrUnsupportedVersion 在 Decrypt 遇到不支持的密文格式时返回
// （如 v2 密文送到 v1 解密器）。
var ErrUnsupportedVersion = errors.New("aesgcm: unsupported ciphertext version")
