// Package httpcache provides an implementation of http.RoundTripper that adds
// transparent HTTP response caching according to RFC 9111 (HTTP Caching).
//
// The main entry point is [NewTransport], which returns an [http.RoundTripper] for use with [http.Client].
// httpcache supports the required standard HTTP caching directives, as well as extension directives such as
// stale-while-revalidate and stale-if-error.
//
// Example usage:
//
//	package main
//
//	import (
//		"log/slog"
//		"net/http"
//		"time"
//
//		"github.com/bartventer/httpcache"
//
//		// Register a cache backend by importing the package
//		_ "github.com/bartventer/httpcache/store/fscache"
//	)
//
//	func main() {
//		dsn := "fscache://?appname=myapp" // // Example DSN for the file system cache backend
//		client := &http.Client{
//			Transport: httpcache.NewTransport(
//				dsn,
//				httpcache.WithSWRTimeout(10*time.Second),
//				httpcache.WithLogger(slog.Default()),
//			),
//		}
//	}
package httpcache

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"net/http"
	"time"

	"github.com/bartventer/httpcache/internal"
	"github.com/bartventer/httpcache/store"
)

const CacheStatusHeader = internal.CacheStatusHeader

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

	rmc   internal.RequestMethodChecker      // Checks if HTTP request methods are understood
	vmc   internal.VaryMatcher               // Matches Vary headers to determine cache validity
	cke   internal.URLKeyer                  // Generates unique cache keys for requests
	fc    internal.FreshnessCalculator       // Calculates the freshness of cached responses
	ce    internal.CacheabilityEvaluator     // Evaluates if a response is cacheable
	siep  internal.StaleIfErrorPolicy        // Handles stale-if-error caching policies
	ci    internal.CacheInvalidator          // Invalidates cache entries based on conditions
	rs    internal.ResponseStorer            // Stores HTTP responses in the cache
	vh    internal.ValidationResponseHandler // Processes validation responses for revalidation
	clock internal.Clock                     // Provides time-related operations, can be mocked for testing
}

const DefaultSWRTimeout = 5 * time.Second // Default timeout for Stale-While-Revalidate

// ErrNilCache is used as the panic value when a nil cache is provided to [NewRoundTripper] or [NewTransport].
// You may recover from this panic if you wish to handle the situation gracefully.
//
// Example usage:
//
//	defer func() {
//		if r := recover(); r != nil {
//			if err, ok := r.(error); ok && errors.Is(err, ErrNilCache) {
//				// Handle the error gracefully, e.g., log it or return a default transport
//				log.Println("Cache cannot be nil:", err)
//				client := &http.Client{
//					Transport: http.DefaultTransport, // Fallback to default transport
//				}
//				// Use the fallback client as needed
//				_ = client
//			} else {
//				// Re-panic if it's not the expected error
//				panic(r)
//			}
//		}
//	}()
var ErrNilCache = errors.New("httpcache: cache cannot be nil")

// NewTransport returns an http.RoundTripper that caches HTTP responses using
// the specified cache backend.
//
// The backend is selected via a DSN (e.g., "memcache://", "fscache://").
// Panics if the cache cannot be opened or is nil. A blank import is required
// to register the cache backend.
//
// To configure the transport, you can use functional options such as
// [WithTransport], [WithSWRTimeout], and [WithLogger].
func NewTransport(dsn string, options ...Option) http.RoundTripper {
	cache, err := store.Open(dsn)
	if err != nil {
		panic(fmt.Errorf("httpcache: failed to open cache: %w", err))
	}
	return newTransport(cache, options...)
}

func newTransport(cache store.Cache, options ...Option) http.RoundTripper {
	if cache == nil {
		panic(ErrNilCache)
	}
	rt := &roundTripper{
		cache: internal.NewResponseCache(cache),
		rmc:   internal.NewRequestMethodChecker(),
		vmc:   internal.NewVaryMatcher(internal.NewHeaderValueNormalizer()),
		cke:   internal.NewURLKeyer(),
		ce:    internal.NewCacheabilityEvaluator(),
		clock: internal.NewClock(),
	}
	rt.fc = internal.NewFreshnessCalculator(rt.clock)
	rt.ci = internal.NewCacheInvalidator(rt.cache, rt.cke)
	rt.siep = internal.NewStaleIfErrorPolicy(rt.clock)
	rt.rs = internal.NewResponseStorer(
		rt.cache,
		internal.NewVaryHeaderNormalizer(),
		internal.NewVaryKeyer(),
	)
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

func (r *roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	urlKey := r.cke.URLKey(req.URL)

	if !r.rmc.IsRequestMethodUnderstood(req) {
		return r.handleUnrecognizedMethod(req, urlKey)
	}

	refs, err := r.cache.GetRefs(urlKey)
	if err != nil || len(refs) == 0 {
		return r.handleCacheMiss(req, urlKey, nil, -1)
	}

	refIdx, found := r.vmc.VaryHeadersMatch(refs, req.Header)
	if !found {
		return r.handleCacheMiss(req, urlKey, refs, -1)
	}

	entry, err := r.cache.Get(refs[refIdx].ResponseID, req)
	if err != nil || entry == nil {
		r.logger.Warn(
			"Cache reference found but entry missing or unreadable; possible cache corruption or concurrent eviction. Falling back to cache miss.",
			slog.String("url", req.URL.String()),
			slog.String("method", req.Method),
			slog.String("cacheKey", urlKey),
			slog.Int("refIndex", refIdx),
			slog.String("vary", refs[refIdx].Vary),
			slog.String("responseID", refs[refIdx].ResponseID),
			slog.Any("error", err),
		)
		return r.handleCacheMiss(req, urlKey, refs, refIdx)
	}

	return r.handleCacheHit(req, entry, urlKey, refs, refIdx)
}

func (r *roundTripper) handleUnrecognizedMethod(
	req *http.Request,
	urlKey string,
) (*http.Response, error) {
	if !internal.IsUnsafeMethod(req) {
		resp, err := r.transport.RoundTrip(req)
		if err != nil {
			return nil, err
		}
		internal.CacheStatusBypass.ApplyTo(resp.Header)
		return resp, nil
	}
	resp, err := r.transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	if internal.IsNonErrorStatus(resp.StatusCode) {
		headers, _ := r.cache.GetRefs(urlKey)
		r.ci.InvalidateCache(req.URL, resp.Header, headers, urlKey)
	}
	internal.CacheStatusBypass.ApplyTo(resp.Header)
	return resp, nil
}

func (r *roundTripper) handleCacheMiss(
	req *http.Request,
	urlKey string,
	headers internal.ResponseRefs,
	refIndex int,
) (*http.Response, error) {
	ccReq := internal.ParseCCRequestDirectives(req.Header)
	if ccReq.OnlyIfCached() {
		return make504Response(req)
	}
	resp, start, end, err := r.roundTripTimed(req)
	if err != nil {
		return nil, err
	}
	ccResp := internal.ParseCCResponseDirectives(resp.Header)
	if r.ce.CanStoreResponse(resp, ccReq, ccResp) {
		_ = r.rs.StoreResponse(resp, urlKey, headers, start, end, refIndex)
	}
	internal.CacheStatusMiss.ApplyTo(resp.Header)
	return resp, nil
}

func (r *roundTripper) handleCacheHit(
	req *http.Request,
	stored *internal.Response,
	urlKey string,
	refs internal.ResponseRefs,
	refIndex int,
) (*http.Response, error) {
	ccReq := internal.ParseCCRequestDirectives(req.Header)
	ccResp := internal.ParseCCResponseDirectives(stored.Data.Header)
	freshness := r.fc.CalculateFreshness(stored, ccReq, ccResp)
	respNoCacheFieldsRaw, hasRespNoCache := ccResp.NoCache()
	respNoCacheFieldsSeq, isRespNoCacheQualified := respNoCacheFieldsRaw.Value()

	// RFC 8246: If response is fresh and immutable, always serve from cache unless request has no-cache
	if !freshness.IsStale && ccResp.Immutable() && !ccReq.NoCache() {
		return r.serveFromCache(stored, freshness, isRespNoCacheQualified, respNoCacheFieldsSeq)
	}

	if freshness.IsStale && ccResp.MustRevalidate() {
		goto revalidate
	}

	// Unqualified no-cache: must revalidate before serving from cache
	if hasRespNoCache && !isRespNoCacheQualified {
		goto revalidate
	}

	if ccReq.OnlyIfCached() || (!freshness.IsStale && !ccReq.NoCache()) {
		return r.serveFromCache(stored, freshness, isRespNoCacheQualified, respNoCacheFieldsSeq)
	}

	if swr, swrValid := ccResp.StaleWhileRevalidate(); freshness.IsStale && swrValid {
		age := freshness.Age.Value + r.clock.Since(freshness.Age.Timestamp)
		staleFor := age - freshness.UsefulLife
		if staleFor >= 0 && staleFor < swr {
			return r.handleStaleWhileRevalidate(req, stored, urlKey, freshness, ccReq)
		}
	}

revalidate:
	req = withConditionalHeaders(req, stored.Data.Header)
	resp, start, end, err := r.roundTripTimed(req)
	ctx := internal.RevalidationContext{
		URLKey:    urlKey,
		Start:     start,
		End:       end,
		CCReq:     ccReq,
		Stored:    stored,
		Refs:      refs,
		RefIndex:  refIndex,
		Freshness: freshness,
	}
	return r.vh.HandleValidationResponse(ctx, req, resp, err)
}

func (r *roundTripper) serveFromCache(
	storedEntry *internal.Response,
	freshness *internal.Freshness,
	noCacheQualified bool,
	noCacheFieldsSeq iter.Seq[string],
) (*http.Response, error) {
	if noCacheQualified {
		//Qualified no-cache: may serve from cache with fields stripped
		for field := range noCacheFieldsSeq {
			storedEntry.Data.Header.Del(field)
		}
	}
	internal.SetAgeHeader(storedEntry.Data, r.clock, freshness.Age)
	internal.CacheStatusHit.ApplyTo(storedEntry.Data.Header)
	return storedEntry.Data, nil
}

func (r *roundTripper) handleStaleWhileRevalidate(
	req *http.Request,
	storedEntry *internal.Response,
	cacheKey string,
	freshness *internal.Freshness,
	ccReq internal.CCRequestDirectives,
) (*http.Response, error) {
	req2 := req.Clone(req.Context())
	req2 = withConditionalHeaders(req2, storedEntry.Data.Header)
	go r.performBackgroundRevalidation(req2, storedEntry, cacheKey, freshness, ccReq)
	internal.CacheStatusStale.ApplyTo(storedEntry.Data.Header)
	return storedEntry.Data, nil
}

func (r *roundTripper) performBackgroundRevalidation(
	req *http.Request,
	storedEntry *internal.Response,
	cacheKey string,
	freshness *internal.Freshness,
	ccReq internal.CCRequestDirectives,
) {
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
}

func (r *roundTripper) backgroundRevalidate(
	req *http.Request,
	stored *internal.Response,
	cacheKey string,
	freshness *internal.Freshness,
	ccReq internal.CCRequestDirectives,
) error {
	//nolint:bodyclose // The response is not used, so we don't need to close it.
	resp, start, end, err := r.roundTripTimed(req)
	if err != nil {
		return fmt.Errorf("background revalidation failed: %w", err)
	}
	if ctxErr := req.Context().Err(); ctxErr != nil {
		return ctxErr
	}
	ctx := internal.RevalidationContext{
		URLKey:    cacheKey,
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
	if resp != nil {
		_ = internal.FixDateHeader(resp.Header, end)
	}
	return
}
