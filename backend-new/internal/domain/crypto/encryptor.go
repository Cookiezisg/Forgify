// Package crypto defines the cross-domain encryption contract used to
// protect sensitive at-rest data — API keys, tokens, webhook secrets.
//
// The package is a domain-level abstraction: it declares WHAT the rest of
// the app can ask of encryption, not HOW it's done. Concrete implementations
// live in infra/crypto.
//
// Why in domain/ (not infra/): "sensitive values must be encrypted before
// persistence" is a business rule — it doesn't depend on AES vs KMS vs HSM.
// The rule stays whether we run on a laptop with a fingerprint-derived key
// or on a cloud fleet backed by AWS KMS. Only the implementation changes.
//
// Package crypto 定义保护敏感持久化数据（API Key、token、webhook secret 等）
// 的跨 domain 加密契约。
//
// 本包是 domain 层的抽象：声明应用其他部分可以**向加密请求什么**，而不是
// **加密如何实现**。具体实现在 infra/crypto。
//
// 为什么放 domain/ 而非 infra/：因为"敏感值持久化前必须加密"是一条**业务规则**，
// 它不随 AES/KMS/HSM 的选型变化。无论跑在笔记本（机器指纹派生密钥）还是
// 云集群（AWS KMS），规则不变，只是实现不同。
package crypto

import "context"

// Encryptor encrypts and decrypts arbitrary byte slices. It is agnostic to
// the content — could be an API Key, OAuth token, webhook secret, or any
// other sensitive value.
//
// Ciphertext format is opaque to callers. Implementations MUST include a
// version tag in their output so the decrypt side can dispatch correctly
// (e.g. "v1:" for local AES-GCM, "v2:" for KMS envelope encryption). This
// lets us migrate algorithms without re-encrypting existing data.
//
// Context is carried through because future implementations (KMS) will make
// network calls; current local implementations may ignore it.
//
// Encryptor 加密和解密任意字节切片。它对内容无感——可以是 API Key、
// OAuth token、webhook secret 或任何其他敏感值。
//
// 密文格式对调用者不透明。实现**必须**在输出中包含版本标识，让解密方能
// 正确分发（如 "v1:" 表示本地 AES-GCM，"v2:" 表示 KMS 信封加密）。这使得
// 算法升级时**无需重加密**已有数据。
//
// 接口带 context 是因为未来的实现（如 KMS）会发网络请求；当前本地实现
// 可以忽略它。
type Encryptor interface {
	// Encrypt seals the plaintext and returns versioned ciphertext safe
	// to store in a TEXT column (ASCII-clean).
	//
	// Encrypt 封装明文并返回带版本标识的密文，保证 ASCII 安全可存 TEXT 列。
	Encrypt(ctx context.Context, plaintext []byte) ([]byte, error)

	// Decrypt reverses Encrypt. It rejects unsupported versions and any
	// malformed ciphertext with a non-nil error — never returns (nil, nil).
	//
	// Decrypt 是 Encrypt 的逆操作。遇到不支持的版本或畸形密文会返回非 nil
	// 错误——永远不会返回 (nil, nil)。
	Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error)
}
