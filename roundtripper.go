// Package httpcache provides an implementation of http.RoundTripper that adds
// transparent HTTP response caching according to RFC 9111 (HTTP Caching).
//
// The main entry point is [NewTransport], which returns an [http.RoundTripper] for use with [http.Client].
// httpcache supports the required standard HTTP caching directives, as well as extension directives such as
// stale-while-revalidate, stale-if-error and immutable.
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
//		dsn := "fscache://?appname=myapp" // Example DSN for the file system cache backend
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
	"github.com/bartventer/httpcache/store/driver"
)

const CacheStatusHeader = internal.CacheStatusHeader

type Option interface {
	apply(*roundTripper)
}

type optionFunc func(*roundTripper)

func (f optionFunc) apply(r *roundTripper) {
	f(r)
}

// WithTransport sets the underlying HTTP transport for making requests.
// Default: http.DefaultTransport.
//
// Note: Any headers added by the provided transport (such as authentication
// headers) will not affect cache key calculation or Vary header matching (per
// RFC 9111 §4.1), as the caching layer operates on the original request as
// received from the client. For example, if you use a transport that adds an
// "Authorization" header, and the stored response has a Vary header that
// includes "Authorization", the cache will not consider this header when
// matching the incoming request to originating requests associated with
// candidate cached responses.
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
	vm    internal.VaryMatcher               // Matches Vary headers to determine cache validity
	uk    internal.URLKeyer                  // Generates unique cache keys for requests
	fc    internal.FreshnessCalculator       // Calculates the freshness of cached responses
	ce    internal.CacheabilityEvaluator     // Evaluates if a response is cacheable
	siep  internal.StaleIfErrorPolicy        // Handles stale-if-error caching policies
	ci    internal.CacheInvalidator          // Invalidates cache entries based on conditions
	rs    internal.ResponseStorer            // Stores HTTP responses in the cache
	vrh   internal.ValidationResponseHandler // Processes validation responses for revalidation
	clock internal.Clock                     // Provides time-related operations, can be mocked for testing
}

const DefaultSWRTimeout = 5 * time.Second // Default timeout for Stale-While-Revalidate

// ErrOpenCache is used as the panic value when the cache cannot be opened.
// You may recover from this panic if you wish to handle the situation gracefully.
//
// Example usage:
//
//	defer func() {
//		if r := recover(); r != nil {
//			if err, ok := r.(error); ok && errors.Is(err, ErrOpenCache) {
//				// Handle the error gracefully, e.g., log it or return a default transport
//				log.Println("Failed to open cache:", err)
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
var ErrOpenCache = errors.New("httpcache: failed to open cache")

// NewTransport returns an http.RoundTripper that caches HTTP responses using
// the specified cache backend.
//
// The backend is selected via a DSN (e.g., "memcache://", "fscache://"), and
// should correlate to a registered cache driver in the [store] package.
// Panics with [ErrOpenCache] if the cache cannot be opened.
//
// To configure the transport, you can use functional options such as
// [WithTransport], [WithSWRTimeout], and [WithLogger].
func NewTransport(dsn string, options ...Option) http.RoundTripper {
	cache, err := store.Open(dsn)
	if err != nil {
		panic(ErrOpenCache)
	}
	return newTransport(cache, options...)
}

func newTransport(conn driver.Conn, options ...Option) http.RoundTripper {
	rt := &roundTripper{
		cache: internal.NewResponseCache(conn),
		rmc:   internal.NewRequestMethodChecker(),
		vm:    internal.NewVaryMatcher(internal.NewHeaderValueNormalizer()),
		uk:    internal.NewURLKeyer(),
		ce:    internal.NewCacheabilityEvaluator(),
		clock: internal.NewClock(),
	}
	rt.fc = internal.NewFreshnessCalculator(rt.clock)
	rt.ci = internal.NewCacheInvalidator(rt.cache, rt.uk)
	rt.siep = internal.NewStaleIfErrorPolicy(rt.clock)
	rt.rs = internal.NewResponseStorer(
		rt.cache,
		internal.NewVaryHeaderNormalizer(),
		internal.NewVaryKeyer(),
	)
	rt.vrh = internal.NewValidationResponseHandler(rt.clock, rt.ci, rt.ce, rt.siep, rt.rs)

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
	urlKey := r.uk.URLKey(req.URL)

	if !r.rmc.IsRequestMethodUnderstood(req) {
		return r.handleUnrecognizedMethod(req, urlKey)
	}

	refs, err := r.cache.GetRefs(urlKey)
	if err != nil || len(refs) == 0 {
		return r.handleCacheMiss(req, urlKey, nil, -1)
	}

	refIndex, found := r.vm.VaryHeadersMatch(refs, req.Header)
	if !found {
		return r.handleCacheMiss(req, urlKey, refs, -1)
	}

	entry, err := r.cache.Get(refs[refIndex].ResponseID, req)
	if err != nil || entry == nil {
		r.logger.Warn(
			"Cache reference found but entry missing or unreadable; possible cache corruption or concurrent eviction. Falling back to cache miss.",
			slog.String("url", req.URL.String()),
			slog.String("method", req.Method),
			slog.String("cacheKey", urlKey),
			slog.Int("refIndex", refIndex),
			slog.String("vary", refs[refIndex].Vary),
			slog.String("responseID", refs[refIndex].ResponseID),
			slog.Any("error", err),
		)
		return r.handleCacheMiss(req, urlKey, refs, refIndex)
	}

	return r.handleCacheHit(req, entry, urlKey, refs, refIndex)
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
		refs, _ := r.cache.GetRefs(urlKey)
		r.ci.InvalidateCache(req.URL, resp.Header, refs, urlKey)
	}
	internal.CacheStatusBypass.ApplyTo(resp.Header)
	return resp, nil
}

func (r *roundTripper) handleCacheMiss(
	req *http.Request,
	urlKey string,
	refs internal.ResponseRefs,
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
		_ = r.rs.StoreResponse(req, resp, urlKey, refs, start, end, refIndex)
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
	return r.vrh.HandleValidationResponse(ctx, req, resp, err)
}

func (r *roundTripper) serveFromCache(
	stored *internal.Response,
	freshness *internal.Freshness,
	noCacheQualified bool,
	noCacheFieldsSeq iter.Seq[string],
) (*http.Response, error) {
	if noCacheQualified {
		//Qualified no-cache: may serve from cache with fields stripped
		for field := range noCacheFieldsSeq {
			stored.Data.Header.Del(field)
		}
	}
	internal.SetAgeHeader(stored.Data, r.clock, freshness.Age)
	internal.CacheStatusHit.ApplyTo(stored.Data.Header)
	return stored.Data, nil
}

func (r *roundTripper) handleStaleWhileRevalidate(
	req *http.Request,
	stored *internal.Response,
	urlKey string,
	freshness *internal.Freshness,
	ccReq internal.CCRequestDirectives,
) (*http.Response, error) {
	req2 := req.Clone(req.Context())
	req2 = withConditionalHeaders(req2, stored.Data.Header)
	go r.performBackgroundRevalidation(req2, stored, urlKey, freshness, ccReq)
	internal.CacheStatusStale.ApplyTo(stored.Data.Header)
	return stored.Data, nil
}

func (r *roundTripper) performBackgroundRevalidation(
	req *http.Request,
	stored *internal.Response,
	urlKey string,
	freshness *internal.Freshness,
	ccReq internal.CCRequestDirectives,
) {
	sl := r.logger.With(
		slog.String("method", req.Method),
		slog.String("url", req.URL.String()),
		slog.String("urlKey", urlKey),
	)
	ctx, cancel := context.WithTimeout(req.Context(), r.swrTimeout)
	defer cancel()
	req = req.WithContext(ctx)
	errc := make(chan error, 1)
	go func() {
		defer close(errc)
		sl.Debug("SWR background revalidation started")
		//nolint:bodyclose // The response is not used, so we don't need to close it.
		resp, start, end, err := r.roundTripTimed(req)
		if err != nil {
			errc <- fmt.Errorf("background revalidation failed: %w", err)
			return
		}
		select {
		case <-req.Context().Done():
			errc <- req.Context().Err()
			return
		default:
		}
		revalCtx := internal.RevalidationContext{
			URLKey:    urlKey,
			Start:     start,
			End:       end,
			CCReq:     ccReq,
			Stored:    stored,
			Freshness: freshness,
		}
		//nolint:bodyclose // The response is not used, so we don't need to close it.
		_, err = r.vrh.HandleValidationResponse(revalCtx, req, resp, nil)
		errc <- err
	}()

	select {
	case <-ctx.Done():
		sl.Debug("SWR background revalidation timeout")
	case swrErr := <-errc:
		if swrErr != nil {
			sl.Error("SWR background revalidation error", slog.Any("error", swrErr))
		} else {
			sl.Debug("SWR background revalidation done")
		}
	}
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
