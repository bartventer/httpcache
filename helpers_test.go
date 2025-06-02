package httpcache

import (
	"net/http"
	"testing"

	"github.com/bartventer/httpcache/internal/testutil"
)

func Test_addConditionalHeaders(t *testing.T) {
	baseReq, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)

	t.Run("without ETag", func(t *testing.T) {
		storedHeaders := make(http.Header)
		storedHeaders.Set("ETag", "foo")
		reqWithHeaders := withConditionalHeaders(baseReq, storedHeaders)
		testutil.AssertNotNil(t, reqWithHeaders)
		testutil.AssertEqual(t, "foo", reqWithHeaders.Header.Get("If-None-Match"))
	})

	t.Run("with Last-Modified", func(t *testing.T) {
		storedHeaders := make(http.Header)
		storedHeaders.Set("Last-Modified", "bar")
		reqWithHeaders := withConditionalHeaders(baseReq, storedHeaders)
		testutil.AssertNotNil(t, reqWithHeaders)
		testutil.AssertEqual(t, "bar", reqWithHeaders.Header.Get("If-Modified-Since"))
	})
}
