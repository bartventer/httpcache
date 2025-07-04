// Copyright (c) 2025 Bart Venter <bartventer@proton.me>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package internal

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/bartventer/httpcache/internal/testutil"
)

func Test_validationResponseHandler_HandleValidationResponse(t *testing.T) {
	noopLogger := NewLogger(slog.DiscardHandler)
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
	storedEntry := &Response{Data: storedResp, RequestedAt: base, ReceivedAt: base}
	ctx := RevalidationContext{
		URLKey:    "key",
		Start:     base,
		End:       base,
		CCReq:     CCRequestDirectives{},
		Stored:    storedEntry,
		Freshness: &Freshness{Age: &Age{Value: 10 * time.Second, Timestamp: base}},
	}

	type args struct {
		req      *http.Request
		resp     *http.Response
		inputErr error
	}

	tests := []struct {
		name    string
		handler *validationResponseHandler
		setup   func(tt *testing.T, handler *validationResponseHandler) args
		assert  func(tt *testing.T, got *http.Response, err error)
	}{
		{
			name: "304 Not Modified",
			handler: &validationResponseHandler{
				l: noopLogger,
			},
			setup: func(tt *testing.T, handler *validationResponseHandler) args {
				return args{
					req:  &http.Request{Method: http.MethodGet},
					resp: &http.Response{StatusCode: http.StatusNotModified, Header: http.Header{}},
				}
			},
			assert: func(tt *testing.T, got *http.Response, err error) {
				testutil.RequireNoError(tt, err)
				testutil.AssertEqual(tt, http.StatusOK, got.StatusCode)
			},
		},
		{
			name: "GET with error status, stale allowed",
			handler: &validationResponseHandler{
				l: noopLogger,
				siep: &MockStaleIfErrorPolicy{
					CanStaleOnErrorFunc: func(*Freshness, ...StaleIfErrorer) bool { return true },
				},
				clock: &MockClock{NowResult: base},
			},
			setup: func(tt *testing.T, handler *validationResponseHandler) args {
				return args{
					req: &http.Request{Method: http.MethodGet},
					resp: &http.Response{
						StatusCode: http.StatusServiceUnavailable,
						Header:     http.Header{"Cache-Control": {"stale-if-error=60"}},
					},
				}
			},
			assert: func(tt *testing.T, got *http.Response, err error) {
				testutil.RequireNoError(tt, err)
				testutil.AssertEqual(tt, http.StatusOK, got.StatusCode)
			},
		},
		{
			name: "GET with error, stale allowed",
			handler: &validationResponseHandler{
				l: noopLogger,
				siep: &MockStaleIfErrorPolicy{
					CanStaleOnErrorFunc: func(*Freshness, ...StaleIfErrorer) bool { return true },
				},
				clock: &MockClock{NowResult: base},
			},
			setup: func(tt *testing.T, handler *validationResponseHandler) args {
				return args{
					req: &http.Request{Method: http.MethodGet},
					resp: &http.Response{
						StatusCode: http.StatusInternalServerError,
						Header:     http.Header{"Cache-Control": {"stale-if-error=60"}},
					},
					inputErr: errors.New("network error"),
				}
			},
			assert: func(tt *testing.T, got *http.Response, err error) {
				testutil.RequireNoError(tt, err)
				testutil.AssertEqual(tt, http.StatusOK, got.StatusCode)
			},
		},
		{
			name: "GET with error, stale not allowed",
			handler: &validationResponseHandler{
				l: noopLogger,
				siep: &MockStaleIfErrorPolicy{
					CanStaleOnErrorFunc: func(*Freshness, ...StaleIfErrorer) bool { return false },
				},
				clock: &MockClock{NowResult: base},
			},
			setup: func(tt *testing.T, handler *validationResponseHandler) args {
				return args{
					req: &http.Request{Method: http.MethodGet},
					resp: &http.Response{
						StatusCode: http.StatusInternalServerError,
						Header:     http.Header{},
					},
					inputErr: errors.New("network error"),
				}
			},
			assert: func(tt *testing.T, got *http.Response, err error) {
				testutil.RequireError(tt, err)
				testutil.AssertNil(tt, got)
			},
		},
		{
			name: "Store response allowed",
			handler: &validationResponseHandler{
				l: noopLogger,
			},
			setup: func(tt *testing.T, handler *validationResponseHandler) args {
				handler.rs = &MockResponseStorer{
					StoreResponseFunc: func(req *http.Request, resp *http.Response, key string, headers ResponseRefs, reqTime, respTime time.Time, refIndex int) error {
						testutil.AssertEqual(tt, "key", key)
						testutil.AssertTrue(tt, respTime.Equal(base))
						testutil.AssertTrue(tt, reqTime.Equal(base))
						return nil
					},
				}
				handler.ce = CacheabilityEvaluatorFunc(
					func(*http.Response, CCRequestDirectives, CCResponseDirectives) bool { return true },
				)
				return args{
					req:  &http.Request{},
					resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{}},
				}
			},
			assert: func(tt *testing.T, got *http.Response, err error) {
				testutil.RequireNoError(tt, err)
			},
		},
		{
			name: "Store response not allowed",
			handler: &validationResponseHandler{
				l: noopLogger,
			},
			setup: func(tt *testing.T, handler *validationResponseHandler) args {
				handler.ci = &MockCacheInvalidator{
					InvalidateCacheFunc: func(reqURL *url.URL, respHeader http.Header, headers ResponseRefs, key string) {
						testutil.AssertEqual(tt, "key", key)
						testutil.AssertTrue(tt, respHeader.Get("Cache-Control") == "")
					},
				}
				handler.ce = CacheabilityEvaluatorFunc(
					func(*http.Response, CCRequestDirectives, CCResponseDirectives) bool { return false },
				)
				handler.rs = &MockResponseStorer{}
				return args{
					req:  &http.Request{},
					resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{}},
				}
			},
			assert: func(tt *testing.T, got *http.Response, err error) {
				testutil.RequireNoError(tt, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := tt.setup(t, tt.handler)
			got, err := tt.handler.HandleValidationResponse(ctx, a.req, a.resp, a.inputErr)
			tt.assert(t, got, err)
		})
	}
}
