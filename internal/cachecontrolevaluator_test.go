package internal

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/bartventer/httpcache/internal/testutil"
)

func Test_canStoreResponse(t *testing.T) {
	type args struct {
		req   *http.Request
		resp  *http.Response
		reqCC CCRequestDirectives
		resCC CCResponseDirectives
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "Response status code not final",
			args: args{
				req:  &http.Request{Method: http.MethodGet},
				resp: &http.Response{StatusCode: http.StatusProcessing},
			},
			want: false,
		},
		{
			name: "Response status not understood",
			args: args{
				req:  &http.Request{Method: http.MethodGet},
				resp: &http.Response{StatusCode: http.StatusPartialContent},
			},
			want: false,
		},
		{
			name: "No-store directive in response",
			args: args{
				req:  &http.Request{Method: http.MethodGet},
				resp: &http.Response{StatusCode: http.StatusOK},
				resCC: CCResponseDirectives{
					"no-store":        "",
					"must-understand": "",
				},
			},
			want: false,
		},
		{
			name: "Valid response for caching",
			args: args{
				req:  &http.Request{Method: http.MethodGet},
				resp: &http.Response{StatusCode: http.StatusNotModified},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.AssertTrue(
				t,
				tt.want == canStoreResponse(tt.args.resp, tt.args.reqCC, tt.args.resCC),
			)
		})
	}
}

func Test_isStatusUnderstood(t *testing.T) {
	for _, code := range []int{
		http.StatusOK,
		http.StatusNonAuthoritativeInfo,
		http.StatusMovedPermanently,
		http.StatusNotModified,
		http.StatusNotFound,
		http.StatusMethodNotAllowed,
		http.StatusGone,
		http.StatusRequestURITooLong,
		http.StatusNotImplemented,
		http.StatusPermanentRedirect,
	} {
		t.Run(fmt.Sprintf("code %d", code), func(t *testing.T) {
			testutil.AssertTrue(t, isStatusUnderstood(code))
		})
	}

	for _, code := range []int{
		http.StatusContinue,
		http.StatusSwitchingProtocols,
		http.StatusProcessing,
		http.StatusPartialContent,
	} {
		t.Run(fmt.Sprintf("code %d", code), func(t *testing.T) {
			testutil.AssertTrue(t, !isStatusUnderstood(code))
		})
	}
}

func Test_isHeuristicallyCacheableCode(t *testing.T) {
	for _, code := range []int{
		http.StatusOK,
		http.StatusNonAuthoritativeInfo,
		http.StatusPartialContent,
		http.StatusMovedPermanently,
		http.StatusNotModified,
		http.StatusNotFound,
		http.StatusMethodNotAllowed,
		http.StatusGone,
		http.StatusRequestURITooLong,
		http.StatusNotImplemented,
		http.StatusPermanentRedirect,
	} {
		t.Run(fmt.Sprintf("code %d", code), func(t *testing.T) {
			testutil.AssertTrue(t, isHeuristicallyCacheableCode(code))
		})
	}

	for _, code := range []int{
		http.StatusContinue,
		http.StatusSwitchingProtocols,
		http.StatusProcessing,
	} {
		t.Run(fmt.Sprintf("code %d", code), func(t *testing.T) {
			testutil.AssertTrue(t, !isHeuristicallyCacheableCode(code))
		})
	}
}

type StaleIfErrorerFunc func() (time.Duration, bool)

func (f StaleIfErrorerFunc) StaleIfError() (time.Duration, bool) {
	return f()
}

func Test_staleIfErrorPolicy_CanStaleOnError(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	type args struct {
		clock     Clock
		freshness *Freshness
		sies      []StaleIfErrorer
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "No StaleIfErrorers",
			want: false,
		},
		{
			name: "Nil StaleIfErrorer",
			args: args{
				sies: []StaleIfErrorer{
					nil, // Should be ignored
				},
			},
			want: false,
		},
		{
			name: "Invalid StaleIfErrorer",
			args: args{
				clock:     &MockClock{SinceResult: 0},
				freshness: &Freshness{},
				sies: []StaleIfErrorer{
					StaleIfErrorerFunc(func() (time.Duration, bool) {
						return 0, false // Invalid StaleIfErrorer
					}),
				},
			},
			want: false,
		},
		{
			name: "StaleIfError with valid duration",
			args: args{
				clock: &MockClock{SinceResult: 0},
				freshness: &Freshness{
					IsStale: true,
					Age: &Age{
						Value:     20 * time.Second,
						Timestamp: base,
					},
					UsefulLife: 15 * time.Second,
				},
				sies: []StaleIfErrorer{
					StaleIfErrorerFunc(func() (time.Duration, bool) {
						return 10 * time.Second, true
					}),
				},
			},
			want: true, // Stale response can be served for 10 seconds after error
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &staleIfErrorPolicy{tt.args.clock}
			got := l.CanStaleOnError(
				tt.args.freshness,
				tt.args.sies...,
			)
			testutil.AssertTrue(t, got == tt.want, "CanStaleOnError mismatch")
		})
	}
}
