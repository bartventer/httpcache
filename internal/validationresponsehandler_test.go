package internal

import (
	"errors"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/bartventer/httpcache/internal/testutil"
)

func Test_validationResponseHandler_HandleValidationResponse(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	storedResp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"ETag":           {"abc"},
			"Last-Modified":  {"yesterday"},
			"Content-Length": {"123"},
		},
		Body: http.NoBody,
	}
	storedEntry := &Entry{Response: storedResp, ReqTime: base, RespTime: base}
	ctx := RevalidationContext{
		CacheKey:  "key",
		Start:     base,
		End:       base,
		CCReq:     CCRequestDirectives{},
		Stored:    storedEntry,
		Freshness: &Freshness{Age: &Age{Value: 10 * time.Second, Timestamp: base}},
	}

	type testCase struct {
		name     string
		handler  *validationResponseHandler
		req      *http.Request
		resp     *http.Response
		inputErr error
		want     *http.Response
		wantErr  bool
		setup    func(*testCase)
		check    func(*testing.T, *testCase, *http.Response)
	}

	tests := []testCase{
		{
			name:    "304 Not Modified",
			handler: &validationResponseHandler{},
			req:     &http.Request{Method: http.MethodGet},
			resp:    &http.Response{StatusCode: http.StatusNotModified, Header: http.Header{}},
			want:    storedResp,
		},
		{
			name:    "HEAD 200, headers match",
			handler: &validationResponseHandler{},
			req:     &http.Request{Method: http.MethodHead},
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"ETag":           {"abc"},
					"Last-Modified":  {"yesterday"},
					"Content-Length": {"123"},
				},
				ContentLength: 123,
			},
			want: storedResp,
		},
		{
			name: "HEAD 200, headers do not match",
			handler: &validationResponseHandler{
				ci: &MockCacheInvalidator{},
			},
			req: &http.Request{Method: http.MethodHead},
			resp: &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"ETag":           {"def"},
					"Last-Modified":  {"today"},
					"Content-Length": {"456"},
				},
				ContentLength: 456,
			},
			want: nil, // will check in check func
			setup: func(tc *testCase) {
				invalidated := false
				tc.handler.ci = &MockCacheInvalidator{
					InvalidateCacheFunc: func(reqURL *url.URL, respHeader http.Header, key string) { invalidated = true },
				}
				tc.check = func(t *testing.T, tc *testCase, got *http.Response) {
					testutil.AssertTrue(t, invalidated)
				}
			},
		},
		{
			name: "GET with error status, stale allowed",
			handler: &validationResponseHandler{
				siep: &MockStaleIfErrorPolicy{
					CanStaleOnErrorFunc: func(freshness *Freshness, sies ...StaleIfErrorer) bool { return true },
				},
				clock: &MockClock{NowResult: base},
			},
			req: &http.Request{Method: http.MethodGet},
			resp: &http.Response{
				StatusCode: http.StatusServiceUnavailable, // Allowed stale error code
				Header:     http.Header{"Cache-Control": {"stale-if-error=60"}},
			},
			inputErr: nil,
			want:     storedResp,
		},
		{
			name: "GET with error, stale allowed",
			handler: &validationResponseHandler{
				siep: &MockStaleIfErrorPolicy{
					CanStaleOnErrorFunc: func(freshness *Freshness, sies ...StaleIfErrorer) bool { return true },
				},
				clock: &MockClock{NowResult: base},
			},
			req: &http.Request{Method: http.MethodGet},
			resp: &http.Response{
				StatusCode: http.StatusInternalServerError,
				Header:     http.Header{"Cache-Control": {"stale-if-error=60"}},
			},
			inputErr: errors.New("network error"), // Simulating an error
			want:     storedResp,
		},
		{
			name: "GET with error, stale not allowed",
			handler: &validationResponseHandler{
				siep: &MockStaleIfErrorPolicy{
					CanStaleOnErrorFunc: func(freshness *Freshness, sies ...StaleIfErrorer) bool { return false },
				},
				clock: &MockClock{NowResult: base},
			},
			req: &http.Request{Method: http.MethodGet},
			resp: &http.Response{
				StatusCode: http.StatusInternalServerError,
				Header:     http.Header{},
			},
			inputErr: errors.New("network error"),
			want:     nil,
			wantErr:  true,
		},
		{
			name: "Store response allowed",
			handler: &validationResponseHandler{
				ce: CacheabilityEvaluatorFunc(
					func(resp *http.Response, reqCC CCRequestDirectives, resCC CCResponseDirectives) bool {
						return true // Simulating a cacheable response
					},
				),
				rs: &MockResponseStorer{},
			},
			req:  &http.Request{},
			resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{}},
			want: nil, // will check in check func
			setup: func(tc *testCase) {
				stored := false
				tc.handler.rs = &MockResponseStorer{
					StoreResponseFunc: func(resp *http.Response, key string, reqTime, respTime time.Time) error {
						stored = true
						testutil.AssertEqual(t, "key", key)
						testutil.AssertTrue(t, respTime.Equal(base))
						testutil.AssertTrue(t, reqTime.Equal(base))
						return nil
					},
				}
				tc.check = func(t *testing.T, tc *testCase, got *http.Response) {
					testutil.AssertTrue(t, stored)
				}
			},
		},
		{
			name: "Store response not allowed",
			handler: &validationResponseHandler{
				ce: CacheabilityEvaluatorFunc(
					func(resp *http.Response, reqCC CCRequestDirectives, resCC CCResponseDirectives) bool {
						return false // Simulating a non-cacheable response
					},
				),
				ci: &MockCacheInvalidator{},
			},
			req:  &http.Request{},
			resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{}},
			want: nil, // will check in check func
			setup: func(tc *testCase) {
				invalidated := false
				tc.handler.ci = &MockCacheInvalidator{
					InvalidateCacheFunc: func(reqURL *url.URL, respHeader http.Header, key string) { invalidated = true },
				}
				tc.check = func(t *testing.T, tc *testCase, got *http.Response) {
					testutil.AssertTrue(t, invalidated)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setup != nil {
				tc.setup(&tc)
			}
			got, err := tc.handler.HandleValidationResponse(ctx, tc.req, tc.resp, tc.inputErr)
			if tc.wantErr {
				testutil.RequireError(t, err)
				testutil.AssertNil(t, got)
			} else {
				testutil.RequireNoError(t, err)
				if tc.check != nil {
					tc.check(t, &tc, got)
				} else {
					testutil.AssertEqual(t, tc.want.StatusCode, got.StatusCode)
					testutil.AssertTrue(t, maps.EqualFunc(tc.want.Header, got.Header, func(a, b []string) bool {
						return slices.Equal(a, b)
					}))
				}
			}
		})
	}
}
