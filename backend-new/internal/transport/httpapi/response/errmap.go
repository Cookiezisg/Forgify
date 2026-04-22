// errmap.go — domain sentinel → HTTP status + stable wire code translation.
//
// Every domain sentinel that should surface meaningfully to the client MUST
// be registered in errTable below. Unregistered errors fall through to
// 500 INTERNAL_ERROR so bugs don't leak implementation details over the wire.
//
// Adding a new domain error:
//  1. Declare the sentinel in domain/<name>/errors.go
//  2. Add a row in errTable mapping it to (status, code)
//  3. Use errors.Is / fmt.Errorf("...: %w", sentinel) to preserve the chain
//
// errmap.go — domain sentinel → HTTP 状态码 + 对外错误码的翻译表。
//
// 每个需要向客户端暴露的 domain sentinel 必须在 errTable 中注册。未注册的
// 错误会降级为 500 INTERNAL_ERROR，防止实现细节经网络泄漏。
//
// 新增 domain 错误的流程：
//  1. 在 domain/<name>/errors.go 中声明 sentinel
//  2. 在 errTable 中加一行映射到 (status, code)
//  3. 使用 errors.Is / fmt.Errorf("...: %w", sentinel) 保持错误链
package response

import (
	stderrors "errors"
	"net/http"

	"go.uber.org/zap"

	derrors "github.com/sunweilin/forgify-new/internal/domain/errors"
)

// errMapping pairs a sentinel with its HTTP status and stable wire code.
//
// errMapping 将 sentinel 与 HTTP 状态码和对外错误码配对。
type errMapping struct {
	Status int
	Code   string
}

// errTable is the single source of truth for domain → HTTP translation.
// Phase 2 will extend this with per-domain rows (tool.ErrNotFound, etc.).
//
// errTable 是 domain → HTTP 翻译的唯一事实源。Phase 2 将补充各 domain 的行
// （tool.ErrNotFound 等）。
var errTable = map[error]errMapping{
	derrors.ErrInvalidRequest: {http.StatusBadRequest, "INVALID_REQUEST"},
	derrors.ErrInternal:       {http.StatusInternalServerError, "INTERNAL_ERROR"},
}

// FromDomainError translates a domain error to an HTTP error envelope.
// It walks errTable with errors.Is so wrapped errors still match.
// Unrecognized errors become 500 INTERNAL_ERROR and are logged — we never
// let an unmapped error leak its raw message to clients.
//
// FromDomainError 把 domain 错误翻译为 HTTP 错误 envelope。用 errors.Is
// 遍历 errTable，使被包裹的错误也能匹配。未识别的错误降级为 500
// INTERNAL_ERROR 并记录日志——绝不让未映射的错误把原始消息泄漏给客户端。
func FromDomainError(w http.ResponseWriter, log *zap.Logger, err error) {
	m, matched := lookup(err)
	msg := err.Error()
	if !matched {
		log.Error("unmapped domain error",
			zap.Error(err),
			zap.String("fallback_code", m.Code),
		)
		// Don't leak the raw error message for unmapped errors.
		// 未映射错误不透出原始消息，避免泄漏实现细节。
		msg = "internal error"
	}
	Error(w, m.Status, m.Code, msg, nil)
}

// lookup returns the mapping for err. If no sentinel matches, it returns
// the INTERNAL_ERROR fallback with matched=false so callers can log.
//
// lookup 返回 err 对应的映射。若无 sentinel 匹配，返回 INTERNAL_ERROR 兜底
// 及 matched=false，调用方可据此决定是否额外记日志。
func lookup(err error) (errMapping, bool) {
	for sentinel, m := range errTable {
		if stderrors.Is(err, sentinel) {
			return m, true
		}
	}
	return errMapping{http.StatusInternalServerError, "INTERNAL_ERROR"}, false
}
