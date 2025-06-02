// Package httpcache provides an implementation of http.RoundTripper that adds
// transparent HTTP response caching according to RFC 9111 (HTTP Caching).
//
// The primary entry point is [NewTransport], which returns an [http.RoundTripper] that can
// be used with [http.Client] to cache HTTP responses in a user-provided Cache.
//
// The package supports standard HTTP caching semantics, including validation,
// freshness calculation, cache revalidation, and support for directives such as
// stale-while-revalidate and stale-if-error.
//
// Example usage:
//
//	client := &http.Client{
//		Transport: httpcache.NewTransport(myCache, httpcache.WithLogger(myLogger)),
//	}
package httpcache

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"time"

	"github.com/bartventer/httpcache/internal"
)

const CacheStatusHeader = internal.CacheStatusHeader

// Option is a functional option for configuring the RoundTripper.
type Option interface {
	apply(*roundTripper)
}

type optionFunc func(*roundTripper)

func (f optionFunc) apply(r *roundTripper) {
	f(r)
}

// WithTransport sets the underlying HTTP transport for making requests;
// default: [http.DefaultTransport].
func WithTransport(transport http.RoundTripper) Option {
	return optionFunc(func(r *roundTripper) {
		r.transport = transport
	})
}

// WithSWRTimeout sets the timeout for Stale-While-Revalidate requests;
// default: [DefaultSWRTimeout].
func WithSWRTimeout(timeout time.Duration) Option {
	return optionFunc(func(r *roundTripper) {
		r.swrTimeout = timeout
	})
}

// WithLogger sets the logger for debug output; default:
// [slog.New]([slog.DiscardHandler]).
func WithLogger(logger *slog.Logger) Option {
	return optionFunc(func(r *roundTripper) {
		r.logger = logger
	})
}

// roundTripper is an implementation of [http.RoundTripper] that caches HTTP responses
// according to the HTTP caching rules defined in RFC 9111.
type roundTripper struct {
	// Configurable options

	cache      internal.ResponseCache // Cache for storing and retrieving responses
	transport  http.RoundTripper      // Underlying HTTP transport for making requests
	swrTimeout time.Duration          // Timeout for Stale-While-Revalidate requests
	logger     *slog.Logger           // Logger for debug output, if needed

	// Internal details

	rmc   internal.RequestMethodChecker      // Request method checker for understanding request methods
	vmc   internal.VaryMatcher               // Vary matcher for checking Vary headers
	cke   internal.CacheKeyer                // Cache keyer for generating cache keys
	fc    internal.FreshnessCalculator       // Freshness calculator for determining freshness of cached responses
	ce    internal.CacheabilityEvaluator     // Cacheability evaluator for determining if a response can be cached
	siep  internal.StaleIfErrorPolicy        // Stale-If-Error policy handler
	ci    internal.CacheInvalidator          // Cache invalidator for handling cache invalidation
	rs    internal.ResponseStorer            // Handler for storing responses in the cache
	vh    internal.ValidationResponseHandler // Handler for validation responses
	clock internal.Clock                     // Clock for time-related operations, can be mocked for testing
}

const DefaultSWRTimeout = 5 * time.Second // Default timeout for Stale-While-Revalidate

// ErrNilCache is returned when a nil cache is provided to [NewRoundTripper].
// Although not recommended, it is possible to handle this error gracefully
// by recovering from the panic that occurs when a nil cache is passed.
//
// For example:
//
//	defer func() {
//		if r := recover(); r != nil {
//			if err, ok := r.(error); ok && errors.Is(err, ErrNilCache) {
//				// Handle the error gracefully, e.g., log it or return a default transport
//				log.Println("Cache cannot be nil:", err)
//			} else {
//				// Re-panic if it's not the expected error
//				panic(r)
//			}
//		}
//	}()
var ErrNilCache = errors.New("httpcache: cache cannot be nil")

// NewTransport creates a new [http.RoundTripper] that caches HTTP responses.
// It requires a non-nil [Cache] implementation to store and retrieve cached responses.
// It also accepts functional options to configure the transport, SWR timeout,
// and logger. If the cache is nil, it panics with [ErrNilCache].
func NewTransport(cache Cache, options ...Option) http.RoundTripper {
	if cache == nil {
		panic(ErrNilCache)
	}
	rt := &roundTripper{
		cache: internal.NewResponseCache(cache),
		// internal detail
		rmc:   internal.NewRequestMethodChecker(),
		vmc:   internal.NewVaryMatcher(),
		cke:   internal.NewCacheKeyer(),
		ce:    internal.NewCacheabilityEvaluator(),
		clock: internal.NewClock(),
	}
	rt.fc = internal.NewFreshnessCalculator(rt.clock)
	rt.ci = internal.NewCacheInvalidator(rt.cache, rt.cke)
	rt.siep = internal.NewStaleIfErrorPolicy(rt.clock)
	rt.rs = internal.NewResponseStorer(rt.cache)
	rt.vh = internal.NewValidationResponseHandler(rt.clock, rt.ci, rt.ce, rt.siep, rt.rs)

	for _, opt := range options {
		opt.apply(rt)
	}
	rt.transport = cmp.Or(rt.transport, http.DefaultTransport)
	rt.swrTimeout = cmp.Or(max(rt.swrTimeout, 0), DefaultSWRTimeout)
	rt.logger = cmp.Or(rt.logger, slog.New(slog.DiscardHandler))
	return rt
}

var _ http.RoundTripper = (*roundTripper)(nil)

//nolint:cyclop,funlen // Cyclomatic complexity is high due to multiple conditions, but it's necessary for RFC compliance.
func (r *roundTripper) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	cacheKey := r.cke.CacheKey(req.URL)

	// 1. Check if method is understood
	//nolint:nestif // Nesting is necessary to handle different request methods and caching logic.
	if !r.rmc.IsRequestMethodUnderstood(req) {
		if !isUnsafeMethod(req) {
			resp, err = r.transport.RoundTrip(req)
			if err != nil {
				return nil, err
			}
			internal.CacheStatusBypass.ApplyTo(resp.Header)
			return resp, nil
		}
		resp, err = r.transport.RoundTrip(req)
		if err != nil {
			return nil, err
		}
		if isNonErrorStatus(resp.StatusCode) {
			r.ci.InvalidateCache(req.URL, resp.Header, cacheKey)
		}
		internal.CacheStatusBypass.ApplyTo(resp.Header)
		return resp, nil
	}

	// 2. Try to get a cached entry
	storedEntry, err := r.cache.Get(cacheKey, req)
	if err != nil || storedEntry == nil ||
		!r.vmc.VaryHeadersMatch(storedEntry.Response.Header, req.Header) {
		ccReq := internal.ParseCCRequestDirectives(req.Header)
		if ccReq.OnlyIfCached() {
			return make504Response(req)
		}
		// Cache miss
		var start, end time.Time
		resp, start, end, err = r.roundTripTimed(req)
		if err != nil {
			return nil, err
		}
		ccResp := internal.ParseCCResponseDirectives(resp.Header)
		if r.ce.CanStoreResponse(resp, ccReq, ccResp) {
			_ = r.rs.StoreResponse(resp, cacheKey, start, end)
		}
		internal.CacheStatusMiss.ApplyTo(resp.Header)
		return resp, nil
	}

	// 3. Cached entry found and Vary matches
	ccReq := internal.ParseCCRequestDirectives(req.Header)
	ccResp := internal.ParseCCResponseDirectives(storedEntry.Response.Header)
	freshness := r.fc.CalculateFreshness(storedEntry, ccReq, ccResp)

	_, ccRespNoCache := ccResp.NoCache()
	if ccReq.OnlyIfCached() || (!freshness.IsStale && !ccReq.NoCache() && !ccRespNoCache) {
		internal.SetAgeHeader(storedEntry.Response, r.clock, freshness.Age)
		internal.CacheStatusHit.ApplyTo(storedEntry.Response.Header)
		return storedEntry.Response, nil
	}

	// 3.1 Handle stale-while-revalidate
	swr, swrValid := ccResp.StaleWhileRevalidate()
	if freshness.IsStale && swrValid {
		age := freshness.Age.Value + r.clock.Since(freshness.Age.Timestamp)
		staleFor := age - freshness.UsefulLife
		if staleFor >= 0 && staleFor < swr {
			req = req.Clone(req.Context())
			req = withConditionalHeaders(req, storedEntry.Response.Header)
			go func() {
				sl := r.logger.With(
					slog.String("method", req.Method),
					slog.String("url", req.URL.String()),
					slog.String("cacheKey", cacheKey),
				)

				ctx, cancel := context.WithTimeout(req.Context(), r.swrTimeout)
				defer cancel()
				req = req.WithContext(ctx)

				done := make(chan struct{})
				var swrErr error
				go func() {
					defer close(done)
					sl.Debug("SWR background revalidation started")
					swrErr = r.backgroundRevalidate(req, storedEntry, cacheKey, freshness, ccReq)
				}()
				select {
				case <-ctx.Done():
					sl.Debug("SWR background revalidation timeout")
				case <-done:
					if swrErr != nil {
						sl.Error("SWR background revalidation error", slog.Any("error", swrErr))
					} else {
						sl.Debug("SWR background revalidation done")
					}
				}
			}()
			internal.CacheStatusStale.ApplyTo(storedEntry.Response.Header)
			return storedEntry.Response, nil
		}
	}

	// 4. Prepare conditional request for revalidation
	req = withConditionalHeaders(req, storedEntry.Response.Header)
	var start, end time.Time
	resp, start, end, err = r.roundTripTimed(req)
	ctx := internal.RevalidationContext{
		CacheKey:  cacheKey,
		Start:     start,
		End:       end,
		CCReq:     ccReq,
		Stored:    storedEntry,
		Freshness: freshness,
	}
	return r.vh.HandleValidationResponse(ctx, req, resp, err)
}

func (r *roundTripper) backgroundRevalidate(
	req *http.Request,
	stored *internal.Entry,
	cacheKey string,
	freshness *internal.Freshness,
	ccReq internal.CCRequestDirectives,
) error {
	var start, end time.Time
	//nolint:bodyclose // The response is not used, so we don't need to close it.
	resp, start, end, err := r.roundTripTimed(req)
	if err != nil {
		return fmt.Errorf("background revalidation failed: %w", err)
	}
	if ctxErr := req.Context().Err(); ctxErr != nil {
		return ctxErr
	}
	ctx := internal.RevalidationContext{
		CacheKey:  cacheKey,
		Start:     start,
		End:       end,
		CCReq:     ccReq,
		Stored:    stored,
		Freshness: freshness,
	}
	//nolint:bodyclose // The response is not used, so we don't need to close it.
	_, err = r.vh.HandleValidationResponse(ctx, req, resp, nil)
	return err
}

func (r *roundTripper) roundTripTimed(
	req *http.Request,
) (resp *http.Response, start, end time.Time, err error) {
	start = r.clock.Now()
	resp, err = r.transport.RoundTrip(req)
	end = r.clock.Now()
	return
}
