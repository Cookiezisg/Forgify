// Package crypto defines the cross-domain encryption contract for at-rest
// sensitive data (API keys, tokens, secrets). Implementations live in
// infra/crypto.
//
// Package crypto 定义保护持久化敏感数据（API Key、token、secret）的
// 加密契约。具体实现在 infra/crypto。
package crypto

import "context"

// Encryptor encrypts/decrypts arbitrary byte slices. Content-agnostic —
// could be an API Key, OAuth token, webhook secret, or anything else.
//
// Ciphertext carries a version tag so multiple algorithms (local AES
// now, KMS envelope later) can coexist during migration.
//
// Encryptor 加密/解密任意字节切片。与内容无关——可以是 API Key、
// OAuth token、webhook secret 等。
//
// 密文带版本标识，让多种算法（目前本地 AES，未来 KMS 信封）在迁移期
// 能共存。
type Encryptor interface {
	// Encrypt seals plaintext and returns versioned ASCII-safe ciphertext.
	//
	// Encrypt 封装明文，返回带版本标识的 ASCII 安全密文。
	Encrypt(ctx context.Context, plaintext []byte) ([]byte, error)

	// Decrypt reverses Encrypt. Rejects unsupported versions or malformed
	// ciphertext with a non-nil error — never returns (nil, nil).
	//
	// Decrypt 是 Encrypt 的逆操作。遇到不支持版本或畸形密文返回非 nil
	// 错误——绝不返回 (nil, nil)。
	Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error)
}
