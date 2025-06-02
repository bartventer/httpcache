package internal

import (
	"net/http"
)

// RequestMethodChecker describes the interface implemented by types that can
// check whether the request method is understood by the cache according to RFC 9111 §3.
type RequestMethodChecker interface {
	IsRequestMethodUnderstood(req *http.Request) bool
}

type RequestMethodCheckerFunc func(req *http.Request) bool

func (f RequestMethodCheckerFunc) IsRequestMethodUnderstood(req *http.Request) bool {
	return f(req)
}

func NewRequestMethodChecker() RequestMethodChecker {
	return RequestMethodCheckerFunc(isRequestMethodUnderstood)
}

func isRequestMethodUnderstood(req *http.Request) bool {
	return (req.Method == http.MethodGet || req.Method == http.MethodHead) &&
		req.Header.Get("Range") == ""
}
