package response

import (
	stderrors "errors"
	"net/http"

	"go.uber.org/zap"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	derrors "github.com/sunweilin/forgify/backend/internal/domain/errors"
)

// errMapping pairs a sentinel with its HTTP status and stable wire code.
//
// errMapping 把 sentinel 错误与 HTTP 状态码和对外错误码配对。
type errMapping struct {
	Status int
	Code   string
}

// errTable is the single source of truth for domain → HTTP translation.
// Adding a new domain error: declare sentinel in domain/<name>/errors.go,
// add a row here, done.
//
// errTable 是 domain → HTTP 翻译的唯一事实源。新增 domain 错误：
// 在 domain/<name>/errors.go 声明 sentinel，在本表加一行即可。
var errTable = map[error]errMapping{
	derrors.ErrInvalidRequest: {http.StatusBadRequest, "INVALID_REQUEST"},
	derrors.ErrInternal:       {http.StatusInternalServerError, "INTERNAL_ERROR"},

	// apikey domain / apikey domain 层
	apikeydomain.ErrNotFound:            {http.StatusNotFound, "API_KEY_NOT_FOUND"},
	apikeydomain.ErrNotFoundForProvider: {http.StatusNotFound, "API_KEY_PROVIDER_NOT_FOUND"},
	apikeydomain.ErrInvalidProvider:     {http.StatusBadRequest, "INVALID_PROVIDER"},
	apikeydomain.ErrBaseURLRequired:     {http.StatusBadRequest, "BASE_URL_REQUIRED"},
	apikeydomain.ErrAPIFormatRequired:   {http.StatusBadRequest, "API_FORMAT_REQUIRED"},
	apikeydomain.ErrKeyRequired:         {http.StatusBadRequest, "KEY_REQUIRED"},
	apikeydomain.ErrTestFailed:          {http.StatusUnprocessableEntity, "API_KEY_TEST_FAILED"},
	apikeydomain.ErrInvalid:             {http.StatusUnauthorized, "API_KEY_INVALID"},
}

// FromDomainError translates a domain error to an HTTP envelope via errTable.
// Unmapped errors → 500 INTERNAL_ERROR; raw message is suppressed to
// prevent leaking implementation details.
//
// FromDomainError 通过 errTable 把 domain 错误翻译为 HTTP envelope。
// 未映射的错误 → 500 INTERNAL_ERROR；原始消息被隐藏，防止泄漏实现细节。
func FromDomainError(w http.ResponseWriter, log *zap.Logger, err error) {
	m, matched := lookup(err)
	msg := err.Error()
	if !matched {
		log.Error("unmapped domain error",
			zap.Error(err),
			zap.String("fallback_code", m.Code),
		)
		msg = "internal error"
	}
	Error(w, m.Status, m.Code, msg, nil)
}

// lookup walks errTable with errors.Is so wrapped errors still match.
//
// lookup 用 errors.Is 遍历 errTable，包裹过的错误也能匹配。
func lookup(err error) (errMapping, bool) {
	for sentinel, m := range errTable {
		if stderrors.Is(err, sentinel) {
			return m, true
		}
	}
	return errMapping{http.StatusInternalServerError, "INTERNAL_ERROR"}, false
}
