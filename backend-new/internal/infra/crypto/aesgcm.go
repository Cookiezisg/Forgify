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
// (e.g. envelope encryption with KMS) will use "v2:", letting the two
// coexist in the database during a lazy re-encryption migration.
//
// v1Prefix 标识 AESGCMEncryptor 产生的密文。未来算法（如基于 KMS 的信封
// 加密）将用 "v2:"，使新旧密文在数据库中共存、支持 lazy 重加密迁移。
const v1Prefix = "v1:"

// AESGCMEncryptor implements domain/crypto.Encryptor using AES-256-GCM with
// a fixed 32-byte master key. Ciphertext format (pre-base64):
//
//	[12-byte nonce][encrypted plaintext + 16-byte GCM tag]
//
// On-wire (written to DB):
//
//	"v1:" + base64(nonce || encrypted || tag)
//
// AESGCMEncryptor 用 AES-256-GCM 加固定 32 字节主密钥实现 domain/crypto.Encryptor。
// 密文格式（base64 前）：
//
//	[12 字节 nonce][加密后的明文 + 16 字节 GCM 认证 tag]
//
// 线上格式（写入 DB）：
//
//	"v1:" + base64(nonce || encrypted || tag)
type AESGCMEncryptor struct {
	gcm cipher.AEAD
}

// NewAESGCMEncryptor constructs an encryptor from a 32-byte master key.
// Key derivation from a machine fingerprint is done via DeriveKey below;
// this function just takes the already-derived key.
//
// NewAESGCMEncryptor 用 32 字节主密钥构造 encryptor。从机器指纹派生密钥的
// 工作由下面的 DeriveKey 完成；本函数只接收已派生好的密钥。
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

// DeriveKey stretches a machine fingerprint (or any entropy source) into a
// 32-byte AES key. A static salt is mixed in so that a leaked fingerprint
// alone cannot reproduce the key without knowing the salt value.
//
// Changing the salt effectively rotates all encryption — existing v1
// ciphertexts become undecryptable. Don't change it casually.
//
// DeriveKey 将机器指纹（或任何熵源）拉伸为 32 字节 AES 密钥。混入静态 salt，
// 确保单靠泄漏的指纹不能重现密钥——还需要知道 salt 值。
//
// 修改 salt 等于轮换所有加密——已有 v1 密文将无法解密。**不要随意改动**。
func DeriveKey(fingerprint string) []byte {
	const salt = "forgify:aesgcm:v1:1ZOI95qH2X" // static salt, do not change / 静态 salt，勿改
	h := sha256.Sum256([]byte(salt + "|" + fingerprint))
	return h[:]
}

// Encrypt implements domain/crypto.Encryptor. Each call generates a fresh
// random nonce so two encryptions of the same plaintext yield different
// ciphertexts (IND-CPA security).
//
// Encrypt 实现 domain/crypto.Encryptor。每次调用生成全新随机 nonce，
// 同一明文两次加密产生不同密文（IND-CPA 安全性）。
func (e *AESGCMEncryptor) Encrypt(_ context.Context, plaintext []byte) ([]byte, error) {
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("aesgcm: generate nonce: %w", err)
	}
	sealed := e.gcm.Seal(nonce, nonce, plaintext, nil)
	encoded := base64.StdEncoding.EncodeToString(sealed)
	return []byte(v1Prefix + encoded), nil
}

// Decrypt implements domain/crypto.Encryptor. It rejects any input that
// doesn't carry the v1 prefix with an ErrUnsupportedVersion error — future
// callers can catch that and try a v2 decryptor.
//
// Decrypt 实现 domain/crypto.Encryptor。遇到无 v1 前缀的输入返回
// ErrUnsupportedVersion——未来调用方可捕获并尝试 v2 解密器。
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
		// Includes authentication failures — wrong key, tampered data, etc.
		// 包含认证失败——错误密钥、被篡改的数据等。
		return nil, fmt.Errorf("aesgcm: open: %w", err)
	}
	return plaintext, nil
}

// ErrUnsupportedVersion is returned when Decrypt sees a ciphertext it wasn't
// designed to handle (e.g. a v2 ciphertext fed to the v1 decryptor).
//
// ErrUnsupportedVersion 在 Decrypt 收到不支持的密文格式时返回（如把 v2 密文
// 送到 v1 解密器）。
var ErrUnsupportedVersion = errors.New("aesgcm: unsupported ciphertext version")
