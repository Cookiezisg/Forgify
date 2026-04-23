// Package apikey organizes LLM credential management into four sub-packages
// by role. The root package has no code — it exists only to document the
// layout.
//
//	types/      — data shapes: APIKey, Credentials, constants, sentinel errors
//	ports/      — interfaces: Repository (infra impls) + KeyProvider (cross-domain)
//	registry/   — whitelist of supported LLM providers and their metadata
//	tools/      — pure helpers (key masking, validators, formatters)
//
// Typical imports from outside this domain:
//
//	"…/domain/apikey/types"   — for *APIKey, sentinel errors
//	"…/domain/apikey/ports"   — for KeyProvider interface (other domains)
//
// Package apikey 把 LLM 凭证管理按角色拆成四个子包。根包**无代码**，
// 仅作结构说明。
//
//	types/      — 数据：APIKey、Credentials、常量、sentinel 错误
//	ports/      — 接口：Repository（给 infra 实现）+ KeyProvider（跨 domain）
//	registry/   — 支持的 LLM provider 白名单及元数据
//	mask/       — key 掩码（用于展示）
package apikey
