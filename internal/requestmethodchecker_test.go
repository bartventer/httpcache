package internal

import (
	"net/http"
	"testing"

	"github.com/bartventer/httpcache/internal/testutil"
)

func Test_isRequestMethodUnderstood(t *testing.T) {
	req := &http.Request{
		Method: http.MethodGet,
		Header: http.Header{},
	}
	got := isRequestMethodUnderstood(req)
	testutil.AssertTrue(t, got)
}
