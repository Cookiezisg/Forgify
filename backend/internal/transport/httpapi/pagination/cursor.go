// Package pagination parses cursor-based pagination params and encodes
// opaque continuation cursors. Cursors are base64url(JSON) so we can
// evolve the shape (e.g. add updated_at later) without bumping the API
// version — clients treat them as opaque.
//
// Package pagination 负责解析 cursor 分页参数并编码不透明的续传 cursor。
// Cursor 是 base64url(JSON)，便于演化内部结构（如未来加 updated_at）
// 而不用升级 API 版本——客户端把它当不透明字符串。
package pagination

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	errorsdomain "github.com/sunweilin/forgify/backend/internal/domain/errors"
)

const (
	DefaultLimit = 50
	MaxLimit     = 200
)

// Params is the normalized pagination input handed to app / infra layers.
//
// Params 是交给 app / infra 层的标准化分页输入。
type Params struct {
	Cursor string
	Limit  int
}

// Parse extracts pagination params from query string. Invalid values
// return errorsdomain.ErrInvalidRequest.
//
// Parse 从 query string 提取分页参数。非法值返回 errorsdomain.ErrInvalidRequest。
func Parse(r *http.Request) (Params, error) {
	q := r.URL.Query()

	limit := DefaultLimit
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return Params{}, fmt.Errorf("limit must be a positive integer: %w", errorsdomain.ErrInvalidRequest)
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

// EncodeCursor marshals v as base64url for the nextCursor field.
// Passing nil yields an empty string (meaning "no more pages").
//
// EncodeCursor 把 v 编码为 base64url，用于 nextCursor 字段。
// 传 nil 得到空字符串（表示"没有下一页"）。
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

// DecodeCursor reverses EncodeCursor. Empty cursor is a no-op (v untouched).
// Malformed cursors return errorsdomain.ErrInvalidRequest.
//
// DecodeCursor 是 EncodeCursor 的逆操作。空 cursor 为 no-op（v 不动）。
// 格式错误的 cursor 返回 errorsdomain.ErrInvalidRequest。
func DecodeCursor(cursor string, v any) error {
	if cursor == "" {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return fmt.Errorf("decode cursor: %w", errorsdomain.ErrInvalidRequest)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("unmarshal cursor: %w", errorsdomain.ErrInvalidRequest)
	}
	return nil
}
