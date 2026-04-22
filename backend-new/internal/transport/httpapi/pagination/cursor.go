// Package pagination parses cursor-based pagination params from requests
// and encodes opaque continuation cursors for responses.
//
// Why cursor-based (N4): offset-based pagination produces duplicate or
// skipped rows under concurrent writes. Cursors pin the pagination to a
// stable column (usually id or (updated_at, id)).
//
// Wire format: clients treat the cursor as an opaque string. Internally it
// is base64url(JSON) so we can evolve the shape (e.g. add updated_at later)
// without bumping the API version — clients neither parse nor construct it.
//
// Package pagination 负责解析请求里的 cursor 分页参数，并为响应编码
// 不透明的续传 cursor。
//
// 为什么用 cursor（N4）：offset 分页在并发写入下会产生重复或跳过的行。
// cursor 把分页锚在稳定列（通常是 id 或 (updated_at, id)）。
//
// 线上格式：客户端把 cursor 当不透明字符串对待。内部是 base64url(JSON)，
// 这样我们以后可以演化形状（如增加 updated_at）而不用升级 API 版本——
// 客户端既不解析也不构造它。
package pagination

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	derrors "github.com/sunweilin/forgify/backend/internal/domain/errors"
)

// Defaults and hard limits applied at the transport layer. Repository code
// should trust the Limit it receives and not re-validate.
//
// transport 层应用的默认值和硬上限。仓储代码应信任收到的 Limit，不应重新校验。
const (
	DefaultLimit = 50
	MaxLimit     = 200
)

// Params is the normalized pagination input handed to app / infra layers.
// Cursor is opaque; use DecodeCursor to read its contents.
//
// Params 是交给 app / infra 层的标准化分页输入。Cursor 是不透明的；
// 用 DecodeCursor 读取其内容。
type Params struct {
	Cursor string
	Limit  int
}

// Parse extracts pagination params from the request query string.
// Missing values use defaults; invalid values return derrors.ErrInvalidRequest
// so the handler can respond 400 INVALID_REQUEST via response.FromDomainError.
//
// Parse 从请求查询字符串提取分页参数。缺失用默认值；非法值返回
// derrors.ErrInvalidRequest，handler 可通过 response.FromDomainError 回 400。
func Parse(r *http.Request) (Params, error) {
	q := r.URL.Query()

	limit := DefaultLimit
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return Params{}, fmt.Errorf("limit must be a positive integer: %w", derrors.ErrInvalidRequest)
		}
		if n > MaxLimit {
			n = MaxLimit
		}
		limit = n
	}

	return Params{
		Cursor: q.Get("cursor"),
		Limit:  limit,
	}, nil
}

// EncodeCursor marshals any JSON-serializable value as a base64url string
// suitable for sending back in the nextCursor field of a Paged response.
// Pass nil or an empty value when there are no more pages.
//
// EncodeCursor 把任意可 JSON 序列化的值编码为 base64url 字符串，可用作
// Paged 响应里 nextCursor 字段的值。没有下一页时传 nil 或空值。
func EncodeCursor(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("encode cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// DecodeCursor reverses EncodeCursor. An empty cursor decodes to a no-op
// (v is left untouched). Malformed cursors return derrors.ErrInvalidRequest.
//
// DecodeCursor 是 EncodeCursor 的逆操作。空 cursor 会原样返回（v 不动）。
// 格式错误的 cursor 返回 derrors.ErrInvalidRequest。
func DecodeCursor(cursor string, v any) error {
	if cursor == "" {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return fmt.Errorf("decode cursor: %w", derrors.ErrInvalidRequest)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("unmarshal cursor: %w", derrors.ErrInvalidRequest)
	}
	return nil
}
