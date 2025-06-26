package httpcache

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/bartventer/httpcache/internal"
	"github.com/bartventer/httpcache/internal/testutil"
	"github.com/bartventer/httpcache/store"
	_ "github.com/bartventer/httpcache/store/memcache"
)

func mockTransport(fields func(rt *transport)) *transport {
	rt := &transport{
		cache:      &internal.MockResponseCache{},
		upstream:   http.DefaultTransport,
		swrTimeout: DefaultSWRTimeout,
		logger:     slog.New(slog.DiscardHandler),
		rmc: &internal.MockRequestMethodChecker{
			IsRequestMethodUnderstoodFunc: func(req *http.Request) bool { return true },
		},
		vm: &internal.MockVaryMatcher{
			VaryHeadersMatchFunc: func(cachedHdrs internal.ResponseRefs, reqHdr http.Header) (int, bool) {
				return 0, true
			},
		},
		uk: &internal.MockCacheKeyer{
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
		vrh: &internal.MockValidationResponseHandler{
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
	if status != expectedStatus.Value {
		t.Errorf("expected cache status %s, got %s", expectedStatus, status)
	}
}

func Test_transport_CacheMissAndStore(t *testing.T) {
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

	rt := mockTransport(func(rt *transport) {
		rt.cache = respCache
		rt.upstream = http.DefaultTransport
		rt.ce = internal.CacheabilityEvaluatorFunc(
			func(resp *http.Response, reqCC internal.CCRequestDirectives, resCC internal.CCResponseDirectives) bool {
				return true // Simulating a cacheable response
			},
		)
		rt.rs = &internal.MockResponseStorer{
			StoreResponseFunc: func(req *http.Request, resp *http.Response, key string, headers internal.ResponseRefs, reqTime, respTime time.Time, refIndex int) error {
				return nil
			},
		}
	})

	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := rt.RoundTrip(req)
	testutil.RequireNoError(t, err)
	testutil.AssertEqual(t, http.StatusOK, resp.StatusCode)
	assertCacheStatus(t, resp, internal.CacheStatusMiss)
}

func Test_transport_CacheHit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not hit origin server on cache hit")
	}))
	defer server.Close()

	storedResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Cache-Control": []string{"max-age=60"}},
		Body:       http.NoBody,
	}
	storedEntry := &internal.Response{
		Data:        storedResp,
		RequestedAt: time.Now(),
		ReceivedAt:  time.Now(),
	}
	mockRespCache := &internal.MockResponseCache{
		GetRefsFunc: func(key string) (internal.ResponseRefs, error) {
			return internal.ResponseRefs{{}}, nil
		},
		GetFunc:    func(key string, req *http.Request) (*internal.Response, error) { return storedEntry, nil },
		SetFunc:    func(key string, entry *internal.Response) error { return nil },
		DeleteFunc: func(key string) error { return nil },
	}

	rt := mockTransport(func(rt *transport) {
		rt.cache = mockRespCache
		rt.upstream = http.DefaultTransport
	})

	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := rt.RoundTrip(req)
	testutil.RequireNoError(t, err)
	testutil.AssertEqual(t, http.StatusOK, resp.StatusCode)
	assertCacheStatus(t, resp, internal.CacheStatusHit)
}

func Test_transport_CacheHit_Immutable(t *testing.T) {
	storedResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Cache-Control": []string{"max-age=60, immutable"}},
		Body:       http.NoBody,
	}
	storedEntry := &internal.Response{
		Data:        storedResp,
		RequestedAt: time.Now(),
		ReceivedAt:  time.Now(),
	}

	rt := mockTransport(func(rt *transport) {
		rt.cache = &internal.MockResponseCache{
			GetFunc: func(key string, req *http.Request) (*internal.Response, error) { return storedEntry, nil },
			GetRefsFunc: func(key string) (internal.ResponseRefs, error) {
				return internal.ResponseRefs{{}}, nil
			},
		}
		rt.fc = &internal.MockFreshnessCalculator{
			CalculateFreshnessFunc: func(resp *http.Response, reqCC internal.CCRequestDirectives, resCC internal.CCResponseDirectives) *internal.Freshness {
				return &internal.Freshness{
					IsStale:    false,
					Age:        &internal.Age{},
					UsefulLife: 60 * time.Second,
				}
			},
		}
	})

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	resp, err := rt.RoundTrip(req)
	testutil.RequireNoError(t, err)
	testutil.AssertNotNil(t, resp)
	testutil.AssertEqual(t, http.StatusOK, resp.StatusCode)
	assertCacheStatus(t, resp, internal.CacheStatusHit)
}

func Test_transport_CacheHit_MustRevalidate_Stale(t *testing.T) {
	storedResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Cache-Control": []string{"max-age=0, must-revalidate"}},
		Body:       http.NoBody,
	}
	storedEntry := &internal.Response{
		Data:        storedResp,
		RequestedAt: time.Now(),
		ReceivedAt:  time.Now(),
	}
	mockVHCalled := false

	rt := mockTransport(func(rt *transport) {
		rt.cache = &internal.MockResponseCache{
			GetFunc: func(key string, req *http.Request) (*internal.Response, error) { return storedEntry, nil },
			GetRefsFunc: func(key string) (internal.ResponseRefs, error) {
				return internal.ResponseRefs{{}}, nil
			},
		}
		rt.fc = &internal.MockFreshnessCalculator{
			CalculateFreshnessFunc: func(resp *http.Response, reqCC internal.CCRequestDirectives, resCC internal.CCResponseDirectives) *internal.Freshness {
				return &internal.Freshness{IsStale: true, Age: &internal.Age{}, UsefulLife: 0}
			},
		}
		rt.vrh = &internal.MockValidationResponseHandler{
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

func Test_transport_CacheHit_NoCacheUnqualified(t *testing.T) {
	storedResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Cache-Control": []string{"max-age=60, no-cache"}},
		Body:       http.NoBody,
	}
	storedEntry := &internal.Response{
		Data:        storedResp,
		RequestedAt: time.Now(),
		ReceivedAt:  time.Now(),
	}
	mockVHCalled := false

	rt := mockTransport(func(rt *transport) {
		rt.cache = &internal.MockResponseCache{
			GetFunc: func(key string, req *http.Request) (*internal.Response, error) { return storedEntry, nil },
			GetRefsFunc: func(key string) (internal.ResponseRefs, error) {
				return internal.ResponseRefs{{}}, nil
			},
		}
		rt.vrh = &internal.MockValidationResponseHandler{
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
func Test_transport_CacheHit_NoCacheQualified_StripsFields(t *testing.T) {
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
	storedEntry := &internal.Response{
		Data:        storedResp,
		RequestedAt: time.Now(),
		ReceivedAt:  time.Now(),
	}

	rt := mockTransport(func(rt *transport) {
		rt.cache = &internal.MockResponseCache{
			GetFunc: func(key string, req *http.Request) (*internal.Response, error) { return storedEntry, nil },
			GetRefsFunc: func(key string) (internal.ResponseRefs, error) {
				return internal.ResponseRefs{{}}, nil
			},
		}
	})

	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	resp, err := rt.RoundTrip(req)
	testutil.RequireNoError(t, err)
	testutil.AssertEqual(t, "", resp.Header.Get("Foo"))
	testutil.AssertEqual(t, "", resp.Header.Get("Bar"))
	testutil.AssertEqual(t, "should-stay", resp.Header.Get("Baz"))
}

func Test_transport_UnrecognizedSafeMethod_Error(t *testing.T) {
	rt := mockTransport(func(rt *transport) {
		rt.rmc = &internal.MockRequestMethodChecker{
			IsRequestMethodUnderstoodFunc: func(req *http.Request) bool { return false },
		}
		rt.upstream = &internal.MockRoundTripper{
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

func Test_transport_NotUnderstoodAndUnsafeMethod(t *testing.T) {
	roundTripperCalled := false
	invalidateCalled := false
	rt := mockTransport(func(rt *transport) {
		rt.cache = &internal.MockResponseCache{
			GetRefsFunc: func(key string) (internal.ResponseRefs, error) {
				return nil, nil
			},
		}
		rt.rmc = &internal.MockRequestMethodChecker{
			IsRequestMethodUnderstoodFunc: func(req *http.Request) bool { return false },
		}
		rt.ci = &internal.MockCacheInvalidator{
			InvalidateCacheFunc: func(reqURL *url.URL, respHeader http.Header, headers internal.ResponseRefs, key string) {
				invalidateCalled = true
			},
		}
		rt.upstream = &internal.MockRoundTripper{
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

func Test_transport_NotUnderstoodAndSafeMethod(t *testing.T) {
	roundTripperCalled := false
	rt := mockTransport(func(rt *transport) {
		rt.rmc = &internal.MockRequestMethodChecker{
			IsRequestMethodUnderstoodFunc: func(req *http.Request) bool { return false },
		}
		rt.upstream = &internal.MockRoundTripper{
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

func Test_transport_NonErrorStatusInvalidation(t *testing.T) {
	invalidateCalled := false
	rt := mockTransport(func(rt *transport) {
		rt.cache = &internal.MockResponseCache{
			GetRefsFunc: func(key string) (internal.ResponseRefs, error) {
				return internal.ResponseRefs{{}}, nil
			},
		}
		rt.rmc = &internal.MockRequestMethodChecker{
			IsRequestMethodUnderstoodFunc: func(req *http.Request) bool { return false },
		}
		rt.upstream = &internal.MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       http.NoBody,
					Header:     make(http.Header),
				}, nil
			},
		}
		rt.ci = &internal.MockCacheInvalidator{
			InvalidateCacheFunc: func(reqURL *url.URL, respHeader http.Header, headers internal.ResponseRefs, key string) {
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

func Test_transport_NotUnderstoodAndRoundTripError(t *testing.T) {
	roundTripperCalled := false
	rt := mockTransport(func(rt *transport) {
		rt.rmc = &internal.MockRequestMethodChecker{
			IsRequestMethodUnderstoodFunc: func(req *http.Request) bool { return false },
		}
		rt.upstream = &internal.MockRoundTripper{
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

func Test_transport_OnlyIfCached504(t *testing.T) {
	rt := mockTransport(func(rt *transport) {
		rt.cache = &internal.MockResponseCache{
			GetFunc: func(key string, req *http.Request) (*internal.Response, error) {
				return nil, errors.New("cache miss")
			},
			GetRefsFunc: func(key string) (internal.ResponseRefs, error) {
				return internal.ResponseRefs{{}}, nil
			},
		}
		rt.vm = &internal.MockVaryMatcher{
			VaryHeadersMatchFunc: func(cachedHdrs internal.ResponseRefs, reqHdr http.Header) (int, bool) {
				return 0, true
			},
		}
	})
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	req.Header.Set("Cache-Control", "only-if-cached")
	resp, err := rt.RoundTrip(req)
	testutil.RequireNoError(t, err)
	testutil.AssertEqual(t, http.StatusGatewayTimeout, resp.StatusCode)
	assertCacheStatus(t, resp, internal.CacheStatusBypass)
}

func Test_transport_CacheMissWithError(t *testing.T) {
	rt := mockTransport(func(rt *transport) {
		rt.cache = &internal.MockResponseCache{
			GetFunc: func(key string, req *http.Request) (*internal.Response, error) {
				return nil, errors.New("cache miss")
			},
			GetRefsFunc: func(key string) (internal.ResponseRefs, error) {
				return internal.ResponseRefs{{}}, nil
			},
		}
		rt.vm = &internal.MockVaryMatcher{
			VaryHeadersMatchFunc: func(cachedHdrs internal.ResponseRefs, reqHdr http.Header) (int, bool) {
				return 0, true
			},
		}
		rt.upstream = &internal.MockRoundTripper{
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

func Test_transport_RevalidationPath(t *testing.T) {
	storedResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Cache-Control": []string{"max-age=0"}},
		Body:       http.NoBody,
	}
	storedEntry := &internal.Response{
		Data:        storedResp,
		RequestedAt: time.Now(),
		ReceivedAt:  time.Now(),
	}
	mockVHCalled := false

	rt := mockTransport(func(rt *transport) {
		rt.cache = &internal.MockResponseCache{
			GetFunc: func(key string, req *http.Request) (*internal.Response, error) { return storedEntry, nil },
			GetRefsFunc: func(key string) (internal.ResponseRefs, error) {
				return internal.ResponseRefs{{}}, nil
			},
		}
		rt.vm = &internal.MockVaryMatcher{
			VaryHeadersMatchFunc: func(cachedHdrs internal.ResponseRefs, reqHdr http.Header) (int, bool) {
				return 0, true
			},
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
		rt.upstream = &internal.MockRoundTripper{
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       http.NoBody,
					Header:     make(http.Header),
				}, nil
			},
		}
		rt.vrh = &internal.MockValidationResponseHandler{
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

func Test_transport_SWR_NormalPath(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	// Simulate a stale cache entry with SWR, normal revalidation path (no timeout).
	storedResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Cache-Control": []string{"max-age=0, stale-while-revalidate=15"}},
		Body:       http.NoBody,
	}
	storedEntry := &internal.Response{
		Data:        storedResp,
		RequestedAt: base.Add(-10 * time.Second),
		ReceivedAt:  base.Add(-10 * time.Second),
	}
	revalidateCalled := make(chan struct{}, 1)
	rt := mockTransport(func(rt *transport) {
		rt.cache = &internal.MockResponseCache{
			GetFunc: func(key string, req *http.Request) (*internal.Response, error) { return storedEntry, nil },
			GetRefsFunc: func(key string) (internal.ResponseRefs, error) {
				return internal.ResponseRefs{{}}, nil
			},
		}
		rt.vm = &internal.MockVaryMatcher{
			VaryHeadersMatchFunc: func(cachedHdrs internal.ResponseRefs, reqHdr http.Header) (int, bool) {
				return 0, true
			},
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
		rt.vrh = &internal.MockValidationResponseHandler{
			HandleValidationResponseFunc: func(ctx internal.RevalidationContext, req *http.Request, resp *http.Response, err error) (*http.Response, error) {
				revalidateCalled <- struct{}{} // Signal that revalidation was called
				return resp, err
			},
		}
		rt.swrTimeout = DefaultSWRTimeout
		rt.upstream = &internal.MockRoundTripper{
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

func Test_transport_SWR_NormalPathAndError(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	// Simulate a stale cache entry with SWR, normal revalidation path with error.
	storedResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Cache-Control": []string{"max-age=0, stale-while-revalidate=15"}},
		Body:       http.NoBody,
	}
	storedEntry := &internal.Response{
		Data:        storedResp,
		RequestedAt: base.Add(-10 * time.Second),
		ReceivedAt:  base.Add(-10 * time.Second),
	}
	swrTimeout := 100 * time.Millisecond

	revalidateCalled := make(chan struct{}, 1)
	rt := mockTransport(func(rt *transport) {
		rt.cache = &internal.MockResponseCache{
			GetFunc: func(key string, req *http.Request) (*internal.Response, error) { return storedEntry, nil },
			GetRefsFunc: func(key string) (internal.ResponseRefs, error) {
				return internal.ResponseRefs{{}}, nil
			},
		}
		rt.vm = &internal.MockVaryMatcher{
			VaryHeadersMatchFunc: func(cachedHdrs internal.ResponseRefs, reqHdr http.Header) (int, bool) {
				return 0, true
			},
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
		rt.vrh = &internal.MockValidationResponseHandler{
			HandleValidationResponseFunc: func(ctx internal.RevalidationContext, req *http.Request, resp *http.Response, err error) (*http.Response, error) {
				defer func() { revalidateCalled <- struct{}{} }() // Signal that revalidation was called
				return nil, errors.New("revalidation error")
			},
		}
		rt.upstream = &internal.MockRoundTripper{
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

func Test_transport_SWR_Timeout(t *testing.T) {
	base := time.Unix(0, 0).UTC()
	// Simulate a stale cache entry with SWR, but timeout before revalidation.
	storedResp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Cache-Control": []string{"max-age=0, stale-while-revalidate=15"}},
		Body:       http.NoBody,
	}
	storedEntry := &internal.Response{
		Data:        storedResp,
		RequestedAt: base.Add(-10 * time.Second),
		ReceivedAt:  base.Add(-10 * time.Second),
	}
	swrTimeout := 50 * time.Millisecond

	revalidateCalled := make(chan struct{}, 1)
	rt := mockTransport(func(rt *transport) {
		rt.cache = &internal.MockResponseCache{
			GetFunc: func(key string, req *http.Request) (*internal.Response, error) { return storedEntry, nil },
			GetRefsFunc: func(key string) (internal.ResponseRefs, error) {
				return internal.ResponseRefs{{}}, nil
			},
		}
		rt.vm = &internal.MockVaryMatcher{
			VaryHeadersMatchFunc: func(cachedHdrs internal.ResponseRefs, reqHdr http.Header) (int, bool) {
				return 0, true
			},
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
		rt.vrh = &internal.MockValidationResponseHandler{
			HandleValidationResponseFunc: func(ctx internal.RevalidationContext, req *http.Request, resp *http.Response, err error) (*http.Response, error) {
				revalidateCalled <- struct{}{} // Signal that revalidation was called
				return resp, err
			},
		}
		rt.upstream = &internal.MockRoundTripper{
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
	rt := newTransport(mockCache, WithUpstream(mockTransport),
		WithLogger(l),
		WithSWRTimeout(swrTimeout),
	)
	testutil.RequireNotNil(t, rt)
	testutil.AssertTrue(t, mockTransport == rt.(*transport).upstream)
	testutil.AssertTrue(t, l == rt.(*transport).logger)
	testutil.AssertEqual(t, swrTimeout, rt.(*transport).swrTimeout)
}

func TestNewTransport_Panic(t *testing.T) {
	testutil.RequirePanics(t, func() {
		NewTransport(
			"invalid-cache-dsn",
			WithUpstream(http.DefaultTransport),
			WithLogger(slog.New(slog.DiscardHandler)),
		)
	})
}

//nolint:cyclop // Acceptable complexity for a test function
func Test_transport_Vary(t *testing.T) {
	etag := `W/"1234567890"`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Vary", "Accept-Language, Accept-Encoding, User-Agent")
		w.Header().Set("Cache-Control", "max-age=60")
		w.Header().Set("ETag", etag)

		if inm := r.Header.Get("If-None-Match"); inm == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		w.WriteHeader(http.StatusOK)
		lang := r.Header.Get("Accept-Language")
		enc := r.Header.Get("Accept-Encoding")
		ua := r.Header.Get("User-Agent")
		switch {
		case lang == "en-us" && enc == "gzip" && ua == "Go-http-client/1.1":
			w.Write([]byte("hello world (en, gzip, go)"))
		case lang == "en-us" && enc == "br" && ua == "Go-http-client/1.1":
			w.Write([]byte("hello world (en, br, go)"))
		case lang == "fr-fr" && enc == "gzip" && ua == "Go-http-client/1.1":
			w.Write([]byte("bonjour le monde (fr, gzip, go)"))
		case lang == "en-us" && enc == "gzip" && ua == "curl/8.0":
			w.Write([]byte("hello world (en, gzip, curl)"))
		default:
			w.Write([]byte("variant"))
		}
	}))
	defer server.Close()
	rt := NewTransport("memcache://")
	drivers := store.Drivers()
	testutil.AssertEqual(t, 1, len(drivers), "expected exactly one driver to be registered")
	for i, tc := range []struct {
		lang, enc, ua, inmatch, wantBody, wantStatus string
	}{
		// Each unique combination should be a MISS first, then HIT
		{"en-us", "gzip", "Go-http-client/1.1", "", "hello world (en, gzip, go)", "MISS"},
		{"en-us", "gzip", "Go-http-client/1.1", etag, "hello world (en, gzip, go)", "HIT"},
		{"en-us", "br", "Go-http-client/1.1", "", "hello world (en, br, go)", "MISS"},
		{"en-us", "br", "Go-http-client/1.1", etag, "hello world (en, br, go)", "HIT"},
		{"fr-fr", "gzip", "Go-http-client/1.1", "", "bonjour le monde (fr, gzip, go)", "MISS"},
		{"fr-fr", "gzip", "Go-http-client/1.1", etag, "bonjour le monde (fr, gzip, go)", "HIT"},
		{"en-us", "gzip", "curl/8.0", "", "hello world (en, gzip, curl)", "MISS"},
		{"en-us", "gzip", "curl/8.0", etag, "hello world (en, gzip, curl)", "HIT"},
	} {
		req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
		req.Header.Set("Accept-Language", tc.lang)
		req.Header.Set("Accept-Encoding", tc.enc)
		req.Header.Set("User-Agent", tc.ua)
		if tc.inmatch != "" {
			req.Header.Set("If-None-Match", tc.inmatch)
		}
		resp, err := rt.RoundTrip(req)
		testutil.RequireNoError(t, err)
		testutil.AssertEqual(t, http.StatusOK, resp.StatusCode, i)
		testutil.AssertEqual(
			t,
			"Accept-Language, Accept-Encoding, User-Agent",
			resp.Header.Get("Vary"),
			i,
		)
		testutil.AssertEqual(t, tc.wantStatus, resp.Header.Get(internal.CacheStatusHeader), i)
		body, _ := io.ReadAll(resp.Body)
		defer resp.Body.Close()
		testutil.AssertEqual(t, tc.wantBody, string(body), i)
	}
}
