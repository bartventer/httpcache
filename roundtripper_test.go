package httpcache

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/bartventer/httpcache/internal"
	"github.com/bartventer/httpcache/internal/testutil"
)

// Helper to create a roundTripper with custom fields for each test.
func newTestRoundTripper(fields func(rt *roundTripper)) *roundTripper {
	rt := &roundTripper{
		cache:      &internal.MockResponseCache{},
		transport:  http.DefaultTransport,
		swrTimeout: DefaultSWRTimeout,
		logger:     slog.New(slog.DiscardHandler),
		rmc: &internal.MockRequestMethodChecker{
			IsRequestMethodUnderstoodFunc: func(req *http.Request) bool { return true },
		},
		vmc: &internal.MockVaryMatcher{
			VaryHeadersMatchFunc: func(_, _ http.Header) bool { return true },
		},
		cke: &internal.MockCacheKeyer{
			CacheKeyFunc: func(u *url.URL) string { return "key" },
		},
		fc: &internal.MockFreshnessCalculator{
			CalculateFreshnessFunc: func(resp *http.Response, reqCC internal.CCRequestDirectives, resCC internal.CCResponseDirectives) *internal.Freshness {
				return &internal.Freshness{
					IsStale:    false,
					Age:        &internal.Age{},
					UsefulLife: 60 * time.Second,
				}
			},
		},
		siep: &internal.MockStaleIfErrorPolicy{},
		ci:   &internal.MockCacheInvalidator{},
		rs:   &internal.MockResponseStorer{},
		vh: &internal.MockValidationResponseHandler{
			HandleValidationResponseFunc: func(ctx internal.RevalidationContext, req *http.Request, resp *http.Response, err error) (*http.Response, error) {
				return resp, err
			},
		},
		clock: &internal.MockClock{NowResult: time.Now()},
	}
	if fields != nil {
		fields(rt)
	}
	return rt
}

func assertCacheStatus(t *testing.T, resp *http.Response, expectedStatus internal.CacheStatus) {
	t.Helper()
	status := resp.Header.Get(internal.CacheStatusHeader)
	if status != expectedStatus.String() {
		t.Errorf("expected cache status %s, got %s", expectedStatus, status)
	}
}

func TestRoundTripper_CacheMissAndStore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=60")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello world"))
	}))
	defer server.Close()

	mockCache := &internal.MockCache{
		GetFunc:    func(key string) ([]byte, error) { return nil, nil },
		SetFunc:    func(key string, entry []byte) error { return nil },
		DeleteFunc: func(key string) error { return nil },
	}
	respCache := internal.NewResponseCache(mockCache)

	rt := newTestRoundTripper(func(rt *roundTripper) {
		rt.cache = respCache
		rt.transport = http.DefaultTransport
		rt.ce = internal.CacheabilityEvaluatorFunc(
			func(resp *http.Response, reqCC internal.CCRequestDirectives, resCC internal.CCResponseDirectives) bool {
				return true // Simulating a cacheable response
			},
		)
		rt.rs = &internal.MockResponseStorer{
			StoreResponseFunc: func(resp *http.Response, key string, reqTime, respTime time.Time) error { return nil },
		}
	})

	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := rt.RoundTrip(req)
	testutil.RequireNoError(t, err)
	testutil.AssertEqual(t, http.StatusOK, resp.StatusCode)
	assertCacheStatus(t, resp, internal.CacheStatusMiss)
}

func TestRoundTripper_CacheHit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not hit origin server on cache hit")
	}))
	defer server.Close()

	storedResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Cache-Control": []string{"max-age=60"}},
		Body:       http.NoBody,
	}
	storedEntry := &internal.Entry{Response: storedResp, ReqTime: time.Now(), RespTime: time.Now()}
	mockRespCache := &internal.MockResponseCache{
		GetFunc:    func(key string, req *http.Request) (*internal.Entry, error) { return storedEntry, nil },
		SetFunc:    func(key string, entry *internal.Entry) error { return nil },
		DeleteFunc: func(key string) error { return nil },
	}

	rt := newTestRoundTripper(func(rt *roundTripper) {
		rt.cache = mockRespCache
		rt.transport = http.DefaultTransport
	})

	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := rt.RoundTrip(req)
	testutil.RequireNoError(t, err)
	testutil.AssertEqual(t, http.StatusOK, resp.StatusCode)
	assertCacheStatus(t, resp, internal.CacheStatusHit)
}

func TestRoundTripper_CacheHit_MustRevalidate_Stale(t *testing.T) {
	storedResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Cache-Control": []string{"max-age=0, must-revalidate"}},
		Body:       http.NoBody,
	}
	storedEntry := &internal.Entry{Response: storedResp, ReqTime: time.Now(), RespTime: time.Now()}
	mockVHCalled := false

	rt := newTestRoundTripper(func(rt *roundTripper) {
		rt.cache = &internal.MockResponseCache{
			GetFunc: func(key string, req *http.Request) (*internal.Entry, error) { return storedEntry, nil },
		}
		rt.fc = &internal.MockFreshnessCalculator{
			CalculateFreshnessFunc: func(resp *http.Response, reqCC internal.CCRequestDirectives, resCC internal.CCResponseDirectives) *internal.Freshness {
				return &internal.Freshness{IsStale: true, Age: &internal.Age{}, UsefulLife: 0}
			},
		}
		rt.vh = &internal.MockValidationResponseHandler{
			HandleValidationResponseFunc: func(ctx internal.RevalidationContext, req *http.Request, resp *http.Response, err error) (*http.Response, error) {
				mockVHCalled = true
				return resp, err
			},
		}
	})

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	resp, err := rt.RoundTrip(req)
	testutil.RequireNoError(t, err)
	testutil.AssertTrue(t, mockVHCalled)
	testutil.AssertNotNil(t, resp)
}

func TestRoundTripper_CacheHit_NoCacheUnqualified(t *testing.T) {
	storedResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Cache-Control": []string{"max-age=60, no-cache"}},
		Body:       http.NoBody,
	}
	storedEntry := &internal.Entry{Response: storedResp, ReqTime: time.Now(), RespTime: time.Now()}
	mockVHCalled := false

	rt := newTestRoundTripper(func(rt *roundTripper) {
		rt.cache = &internal.MockResponseCache{
			GetFunc: func(key string, req *http.Request) (*internal.Entry, error) { return storedEntry, nil },
		}
		rt.vh = &internal.MockValidationResponseHandler{
			HandleValidationResponseFunc: func(ctx internal.RevalidationContext, req *http.Request, resp *http.Response, err error) (*http.Response, error) {
				mockVHCalled = true
				return resp, err
			},
		}
	})

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	resp, err := rt.RoundTrip(req)
	testutil.RequireNoError(t, err)
	testutil.AssertTrue(t, mockVHCalled)
	testutil.AssertNotNil(t, resp)
}
func TestRoundTripper_CacheHit_NoCacheQualified_StripsFields(t *testing.T) {
	storedResp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Cache-Control": []string{`max-age=60, no-cache="Foo,Bar"`},
			"Foo":           []string{"should-be-removed"},
			"Bar":           []string{"should-be-removed"},
			"Baz":           []string{"should-stay"},
		},
		Body: http.NoBody,
	}
	storedEntry := &internal.Entry{Response: storedResp, ReqTime: time.Now(), RespTime: time.Now()}

	rt := newTestRoundTripper(func(rt *roundTripper) {
		rt.cache = &internal.MockResponseCache{
			GetFunc: func(key string, req *http.Request) (*internal.Entry, error) { return storedEntry, nil },
		}
	})

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	resp, err := rt.RoundTrip(req)
	testutil.RequireNoError(t, err)
	testutil.AssertEqual(t, "", resp.Header.Get("Foo"))
	testutil.AssertEqual(t, "", resp.Header.Get("Bar"))
	testutil.AssertEqual(t, "should-stay", resp.Header.Get("Baz"))
}

func TestRoundTripper_UnrecognizedSafeMethod_Error(t *testing.T) {
	rt := newTestRoundTripper(func(rt *roundTripper) {
		rt.rmc = &internal.MockRequestMethodChecker{
			IsRequestMethodUnderstoodFunc: func(req *http.Request) bool { return false },
		}
		rt.transport = &internal.MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				return nil, testutil.ErrSample
			},
		}
	})
	req, _ := http.NewRequest(http.MethodTrace, "http://example.com", nil) // TRACE is a safe method
	resp, err := rt.RoundTrip(req)
	testutil.RequireErrorIs(t, err, testutil.ErrSample)
	testutil.AssertNil(t, resp)
}

func TestRoundTripper_NotUnderstoodAndUnsafeMethod(t *testing.T) {
	roundTripperCalled := false
	invalidateCalled := false
	rt := newTestRoundTripper(func(rt *roundTripper) {
		rt.rmc = &internal.MockRequestMethodChecker{
			IsRequestMethodUnderstoodFunc: func(req *http.Request) bool { return false },
		}
		rt.ci = &internal.MockCacheInvalidator{
			InvalidateCacheFunc: func(reqURL *url.URL, respHeader http.Header, key string) {
				invalidateCalled = true
			},
		}
		rt.transport = &internal.MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				roundTripperCalled = true
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       http.NoBody,
					Header:     make(http.Header),
				}, nil
			},
		}
	})
	req, _ := http.NewRequest(http.MethodDelete, "http://example.com", nil)
	resp, err := rt.RoundTrip(req)
	testutil.RequireNoError(t, err)
	testutil.AssertTrue(t, roundTripperCalled)
	testutil.AssertTrue(t, invalidateCalled)
	testutil.AssertEqual(t, http.StatusOK, resp.StatusCode)
	assertCacheStatus(t, resp, internal.CacheStatusBypass)
}

func TestRoundTripper_NotUnderstoodAndSafeMethod(t *testing.T) {
	roundTripperCalled := false
	rt := newTestRoundTripper(func(rt *roundTripper) {
		rt.rmc = &internal.MockRequestMethodChecker{
			IsRequestMethodUnderstoodFunc: func(req *http.Request) bool { return false },
		}
		rt.transport = &internal.MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				roundTripperCalled = true
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       http.NoBody,
					Header:     make(http.Header),
				}, nil
			},
		}
	})
	req, _ := http.NewRequest(http.MethodTrace, "http://example.com", nil)
	resp, err := rt.RoundTrip(req)
	testutil.RequireNoError(t, err)
	testutil.AssertTrue(t, roundTripperCalled)
	testutil.AssertEqual(t, http.StatusOK, resp.StatusCode)
	assertCacheStatus(t, resp, internal.CacheStatusBypass)
}

func TestRoundTripper_NonErrorStatusInvalidation(t *testing.T) {
	invalidateCalled := false
	rt := newTestRoundTripper(func(rt *roundTripper) {
		rt.rmc = &internal.MockRequestMethodChecker{
			IsRequestMethodUnderstoodFunc: func(req *http.Request) bool { return false },
		}
		rt.transport = &internal.MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       http.NoBody,
					Header:     make(http.Header),
				}, nil
			},
		}
		rt.ci = &internal.MockCacheInvalidator{
			InvalidateCacheFunc: func(reqURL *url.URL, respHeader http.Header, key string) {
				invalidateCalled = true
			},
		}
	})
	req, _ := http.NewRequest(http.MethodDelete, "http://example.com", nil)
	resp, err := rt.RoundTrip(req)
	testutil.RequireNoError(t, err)
	testutil.AssertTrue(t, invalidateCalled)
	testutil.AssertEqual(t, http.StatusOK, resp.StatusCode)
	assertCacheStatus(t, resp, internal.CacheStatusBypass)
}

func TestRoundTripper_NotUnderstoodAndRoundTripError(t *testing.T) {
	roundTripperCalled := false
	rt := newTestRoundTripper(func(rt *roundTripper) {
		rt.rmc = &internal.MockRequestMethodChecker{
			IsRequestMethodUnderstoodFunc: func(req *http.Request) bool { return false },
		}
		rt.transport = &internal.MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				roundTripperCalled = true
				return nil, testutil.ErrSample
			},
		}
	})
	req, _ := http.NewRequest(http.MethodDelete, "http://example.com", nil)
	resp, err := rt.RoundTrip(req)
	testutil.RequireErrorIs(t, err, testutil.ErrSample)
	testutil.AssertNil(t, resp)
	testutil.AssertTrue(t, roundTripperCalled)
}

func TestRoundTripper_OnlyIfCached504(t *testing.T) {
	rt := newTestRoundTripper(func(rt *roundTripper) {
		rt.cache = &internal.MockResponseCache{
			GetFunc: func(key string, req *http.Request) (*internal.Entry, error) { return nil, errors.New("cache miss") },
		}
		rt.vmc = &internal.MockVaryMatcher{
			VaryHeadersMatchFunc: func(_, _ http.Header) bool { return true },
		}
	})
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	req.Header.Set("Cache-Control", "only-if-cached")
	resp, err := rt.RoundTrip(req)
	testutil.RequireNoError(t, err)
	testutil.AssertEqual(t, http.StatusGatewayTimeout, resp.StatusCode)
	assertCacheStatus(t, resp, internal.CacheStatusBypass)
}

func TestRoundTripper_CacheMissWithError(t *testing.T) {
	rt := newTestRoundTripper(func(rt *roundTripper) {
		rt.cache = &internal.MockResponseCache{
			GetFunc: func(key string, req *http.Request) (*internal.Entry, error) { return nil, errors.New("cache miss") },
		}
		rt.vmc = &internal.MockVaryMatcher{
			VaryHeadersMatchFunc: func(_, _ http.Header) bool { return true },
		}
		rt.transport = &internal.MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				return nil, testutil.ErrSample
			},
		}
	})
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	resp, err := rt.RoundTrip(req)
	testutil.RequireErrorIs(t, err, testutil.ErrSample)
	testutil.AssertNil(t, resp)
}

func TestRoundTripper_RevalidationPath(t *testing.T) {
	storedResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Cache-Control": []string{"max-age=0"}},
		Body:       http.NoBody,
	}
	storedEntry := &internal.Entry{Response: storedResp, ReqTime: time.Now(), RespTime: time.Now()}
	mockVHCalled := false

	rt := newTestRoundTripper(func(rt *roundTripper) {
		rt.cache = &internal.MockResponseCache{
			GetFunc: func(key string, req *http.Request) (*internal.Entry, error) { return storedEntry, nil },
		}
		rt.vmc = &internal.MockVaryMatcher{
			VaryHeadersMatchFunc: func(_, _ http.Header) bool { return true },
		}
		rt.fc = &internal.MockFreshnessCalculator{
			CalculateFreshnessFunc: func(resp *http.Response, reqCC internal.CCRequestDirectives, resCC internal.CCResponseDirectives) *internal.Freshness {
				return &internal.Freshness{
					IsStale: true,
					Age: &internal.Age{
						Value:     10 * time.Second,
						Timestamp: time.Now().Add(-10 * time.Second),
					},
					UsefulLife: 0,
				}
			},
		}
		rt.clock = &internal.MockClock{NowResult: time.Now()}
		rt.siep = &internal.MockStaleIfErrorPolicy{}
		rt.transport = &internal.MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       http.NoBody,
					Header:     make(http.Header),
				}, nil
			},
		}
		rt.vh = &internal.MockValidationResponseHandler{
			HandleValidationResponseFunc: func(ctx internal.RevalidationContext, req *http.Request, resp *http.Response, err error) (*http.Response, error) {
				mockVHCalled = true
				internal.CacheStatusRevalidated.ApplyTo(resp.Header)
				return resp, err
			},
		}
	})

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	resp, err := rt.RoundTrip(req)
	testutil.RequireNoError(t, err)
	testutil.AssertEqual(t, http.StatusOK, resp.StatusCode)
	testutil.AssertTrue(t, mockVHCalled)
	assertCacheStatus(t, resp, internal.CacheStatusRevalidated)
}

func TestRoundTripper_SWR_NormalPath(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	// Simulate a stale cache entry with SWR, normal revalidation path (no timeout).
	storedResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Cache-Control": []string{"max-age=0, stale-while-revalidate=15"}},
		Body:       http.NoBody,
	}
	storedEntry := &internal.Entry{
		Response: storedResp,
		ReqTime:  base.Add(-10 * time.Second),
		RespTime: base.Add(-10 * time.Second),
	}
	revalidateCalled := make(chan struct{}, 1)
	rt := newTestRoundTripper(func(rt *roundTripper) {
		rt.cache = &internal.MockResponseCache{
			GetFunc: func(key string, req *http.Request) (*internal.Entry, error) { return storedEntry, nil },
		}
		rt.vmc = &internal.MockVaryMatcher{
			VaryHeadersMatchFunc: func(_, _ http.Header) bool { return true },
		}
		rt.fc = &internal.MockFreshnessCalculator{
			CalculateFreshnessFunc: func(resp *http.Response, reqCC internal.CCRequestDirectives, resCC internal.CCResponseDirectives) *internal.Freshness {
				return &internal.Freshness{
					IsStale: true,
					Age: &internal.Age{
						Value:     10 * time.Second,
						Timestamp: base.Add(-10 * time.Second),
					},
					UsefulLife: 0,
				}
			},
		}
		rt.clock = &internal.MockClock{NowResult: base.Add(5 * time.Second), SinceResult: 0}
		rt.siep = &internal.MockStaleIfErrorPolicy{}
		rt.vh = &internal.MockValidationResponseHandler{
			HandleValidationResponseFunc: func(ctx internal.RevalidationContext, req *http.Request, resp *http.Response, err error) (*http.Response, error) {
				revalidateCalled <- struct{}{} // Signal that revalidation was called
				return resp, err
			},
		}
		rt.swrTimeout = DefaultSWRTimeout
		rt.transport = &internal.MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       http.NoBody,
					Header:     http.Header{},
				}, nil
			},
		}
	})

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	resp, err := rt.RoundTrip(req)
	testutil.RequireNoError(t, err)
	testutil.AssertEqual(t, http.StatusOK, resp.StatusCode)
	assertCacheStatus(t, resp, internal.CacheStatusStale)
	select {
	case <-revalidateCalled:
		// Success: background revalidate called
	case <-time.After(DefaultSWRTimeout):
		t.Error("expected background revalidate to be called")
	}
}

func TestRoundTripper_SWR_NormalPathAndError(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	// Simulate a stale cache entry with SWR, normal revalidation path with error.
	storedResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Cache-Control": []string{"max-age=0, stale-while-revalidate=15"}},
		Body:       http.NoBody,
	}
	storedEntry := &internal.Entry{
		Response: storedResp,
		ReqTime:  base.Add(-10 * time.Second),
		RespTime: base.Add(-10 * time.Second),
	}
	swrTimeout := 100 * time.Millisecond

	revalidateCalled := make(chan struct{}, 1)
	rt := newTestRoundTripper(func(rt *roundTripper) {
		rt.cache = &internal.MockResponseCache{
			GetFunc: func(key string, req *http.Request) (*internal.Entry, error) { return storedEntry, nil },
		}
		rt.vmc = &internal.MockVaryMatcher{
			VaryHeadersMatchFunc: func(_, _ http.Header) bool { return true },
		}
		rt.fc = &internal.MockFreshnessCalculator{
			CalculateFreshnessFunc: func(resp *http.Response, reqCC internal.CCRequestDirectives, resCC internal.CCResponseDirectives) *internal.Freshness {
				return &internal.Freshness{
					IsStale: true,
					Age: &internal.Age{
						Value:     10 * time.Second,
						Timestamp: base.Add(-10 * time.Second),
					},
					UsefulLife: 0,
				}
			},
		}
		rt.clock = &internal.MockClock{NowResult: base.Add(5 * time.Second), SinceResult: 0}
		rt.swrTimeout = swrTimeout
		rt.vh = &internal.MockValidationResponseHandler{
			HandleValidationResponseFunc: func(ctx internal.RevalidationContext, req *http.Request, resp *http.Response, err error) (*http.Response, error) {
				defer func() { revalidateCalled <- struct{}{} }() // Signal that revalidation was called
				return nil, errors.New("revalidation error")
			},
		}
		rt.transport = &internal.MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				return nil, errors.New("network error") // Simulate an error during revalidation
			},
		}
	})
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	resp, err := rt.RoundTrip(req)
	testutil.RequireNoError(t, err)
	testutil.AssertEqual(t, http.StatusOK, resp.StatusCode)
	assertCacheStatus(t, resp, internal.CacheStatusStale)
	select {
	case <-revalidateCalled:
		t.Error("expected background revalidate to not be called due to error")
	case <-time.After(swrTimeout + 100*time.Millisecond):
		// Success: revalidate was not called due to error
	}
}

func TestRoundTripper_SWR_Timeout(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	// Simulate a stale cache entry with SWR, but timeout before revalidation.
	storedResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Cache-Control": []string{"max-age=0, stale-while-revalidate=15"}},
		Body:       http.NoBody,
	}
	storedEntry := &internal.Entry{
		Response: storedResp,
		ReqTime:  base.Add(-10 * time.Second),
		RespTime: base.Add(-10 * time.Second),
	}
	swrTimeout := 50 * time.Millisecond

	revalidateCalled := make(chan struct{}, 1)
	rt := newTestRoundTripper(func(rt *roundTripper) {
		rt.cache = &internal.MockResponseCache{
			GetFunc: func(key string, req *http.Request) (*internal.Entry, error) { return storedEntry, nil },
		}
		rt.vmc = &internal.MockVaryMatcher{
			VaryHeadersMatchFunc: func(_, _ http.Header) bool { return true },
		}
		rt.fc = &internal.MockFreshnessCalculator{
			CalculateFreshnessFunc: func(resp *http.Response, reqCC internal.CCRequestDirectives, resCC internal.CCResponseDirectives) *internal.Freshness {
				return &internal.Freshness{
					IsStale: true,
					Age: &internal.Age{
						Value:     10 * time.Second,
						Timestamp: base.Add(-10 * time.Second),
					},
					UsefulLife: 0,
				}
			},
		}
		rt.clock = &internal.MockClock{NowResult: base.Add(5 * time.Second), SinceResult: 0}
		rt.swrTimeout = swrTimeout
		rt.vh = &internal.MockValidationResponseHandler{
			HandleValidationResponseFunc: func(ctx internal.RevalidationContext, req *http.Request, resp *http.Response, err error) (*http.Response, error) {
				revalidateCalled <- struct{}{} // Signal that revalidation was called
				return resp, err
			},
		}
		rt.transport = &internal.MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				time.Sleep(swrTimeout + 500*time.Millisecond) // Simulate long revalidation
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       http.NoBody,
					Header:     http.Header{},
				}, nil
			},
		}
	})

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	resp, err := rt.RoundTrip(req)
	testutil.RequireNoError(t, err)
	testutil.AssertEqual(t, http.StatusOK, resp.StatusCode)
	assertCacheStatus(t, resp, internal.CacheStatusStale)

	select {
	case <-revalidateCalled:
		t.Error("expected background revalidate to not be called due to timeout")
	case <-time.After(swrTimeout + 750*time.Millisecond):
		// Success: revalidate was not called due to timeout
	}
}

func Test_newTransport(t *testing.T) {
	mockTransport := &internal.MockRoundTripper{}
	l := slog.New(slog.DiscardHandler)
	swrTimeout := 100 * time.Millisecond
	mockCache := &internal.MockCache{}
	rt := newTransport(mockCache, WithTransport(mockTransport),
		WithLogger(l),
		WithSWRTimeout(swrTimeout),
	)
	testutil.RequireNotNil(t, rt)
	testutil.AssertTrue(t, mockTransport == rt.(*roundTripper).transport)
	testutil.AssertTrue(t, l == rt.(*roundTripper).logger)
	testutil.AssertEqual(t, swrTimeout, rt.(*roundTripper).swrTimeout)
}

func Test_newTransport_Panic(t *testing.T) {
	panicked := false
	defer func() {
		if r := recover(); r != nil {
			panicked = true
		}
	}()
	newTransport(
		nil,
		WithTransport(http.DefaultTransport),
		WithLogger(slog.New(slog.DiscardHandler)),
	)
	testutil.AssertTrue(t, panicked, "expected panic when cache is nil")
}
