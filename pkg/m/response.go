// Package model contains transport-independent technical contracts.
// It is deliberately not a shared collection of service domain entities.
package m

type JsonResult struct {
	Code int         `json:"code"`
	Err  string      `json:"err,omitempty"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data,omitempty"`
}

type ApiError struct {
	Status   int    `json:"status"`
	Code     int    `json:"code"`
	Langkey  string `json:"langkey"`
	Message  string `json:"msg"`
	Moreinfo string `json:"moreinfo"`
	Err      error  `json:"err"`
}

func NewApiError(status, code int, langKey, message, info string, err error) *ApiError {
	return &ApiError{status, code, langKey, message, info, err}
}

func (err *ApiError) Error() string { return err.Message }

// WithCause returns a copy, so a shared error template is never mutated.
// Legacy r.ProcErr intentionally retains its original in-place semantics.
func (err *ApiError) WithCause(cause error) *ApiError {
	result := *err
	result.Err = cause
	return &result
}

type ValidationError struct {
	ActualTag string `json:"tag"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Type      string `json:"type"`
	Value     string `json:"value"`
	Param     string `json:"param"`
}

type PageInfo[T any] struct {
	CurrentPage int   `json:"currentPage"`
	PageSize    int   `json:"pageSize"`
	TotalPages  int   `json:"totalPages"`
	TotalItems  int64 `json:"count"`
	HasNextPage bool  `json:"hasNextPage"`
	Filter      any   `json:"filter"`
	Items       []T   `json:"list"`
}
