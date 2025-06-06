package internal

import (
	"net/http"
	"time"
)

const CacheStatusHeader = "X-Httpcache-Status"

type CacheStatus string

// ApplyTo sets the cache status header on the provided HTTP header.
func (s CacheStatus) ApplyTo(header http.Header) { header.Set(CacheStatusHeader, s.String()) }
func (s CacheStatus) String() string             { return string(s) }

const (
	CacheStatusHit         CacheStatus = "HIT"         // Response was served from cache
	CacheStatusMiss        CacheStatus = "MISS"        // Response was not found in cache, and was served from origin
	CacheStatusStale       CacheStatus = "STALE"       // Response was served from cache but is stale
	CacheStatusRevalidated CacheStatus = "REVALIDATED" // Response was revalidated with the origin server
	CacheStatusBypass      CacheStatus = "BYPASS"      // Response was not served from cache due to cache bypass
)

type HeaderKey string

func (h HeaderKey) String() string                { return string(h) }
func (h HeaderKey) Equal(h1, h2 http.Header) bool { return h1.Get(string(h)) == h2.Get(string(h)) }

type ValidationResponseHandler interface {
	HandleValidationResponse(
		ctx RevalidationContext,
		req *http.Request,
		resp *http.Response,
		err error,
	) (*http.Response, error)
}

type RevalidationContext struct {
	CacheKey   string
	Start, End time.Time
	CCReq      CCRequestDirectives
	Stored     *Entry
	Freshness  *Freshness
}

type validationResponseHandler struct {
	clock Clock
	ci    CacheInvalidator
	ce    CacheabilityEvaluator
	siep  StaleIfErrorPolicy
	rs    ResponseStorer
}

func NewValidationResponseHandler(
	clock Clock,
	ci CacheInvalidator,
	ce CacheabilityEvaluator,
	siep StaleIfErrorPolicy,
	rs ResponseStorer,
) *validationResponseHandler {
	return &validationResponseHandler{clock, ci, ce, siep, rs}
}

//nolint:cyclop // Complexity is high due to RFC 9111 rules.
func (r *validationResponseHandler) HandleValidationResponse(
	ctx RevalidationContext,
	req *http.Request,
	resp *http.Response,
	err error,
) (*http.Response, error) {
	switch {
	case err == nil && req.Method == http.MethodGet && resp.StatusCode == http.StatusNotModified:
		// RFC 9111 §4.3.3 Handling Validation Responses (304 Not Modified)
		// RFC 9111 §4.3.4 Freshening Stored Responses upon Validation
		updateStoredHeaders(ctx.Stored.Response, resp)
		CacheStatusRevalidated.ApplyTo(ctx.Stored.Response.Header)
		return ctx.Stored.Response, nil
	case err == nil && req.Method == http.MethodHead && resp.StatusCode == http.StatusOK:
		// RFC 9111 §4.3.5 Freshening Responses with HEAD
		if HeaderKey("ETag").Equal(ctx.Stored.Response.Header, resp.Header) &&
			HeaderKey("Last-Modified").Equal(ctx.Stored.Response.Header, resp.Header) &&
			HeaderKey("Content-Length").Equal(ctx.Stored.Response.Header, resp.Header) {
			updateStoredHeaders(ctx.Stored.Response, resp)
			CacheStatusRevalidated.ApplyTo(resp.Header)
		} else {
			r.ci.InvalidateCache(req.URL, resp.Header, ctx.CacheKey)
			CacheStatusBypass.ApplyTo(resp.Header)
		}
		return resp, nil
	case (err != nil || isStaleErrorAllowed(resp.StatusCode)) &&
		req.Method == http.MethodGet &&
		r.siep.CanStaleOnError(ctx.Freshness, ParseCCResponseDirectives(resp.Header)):
		// RFC 9111 §4.2.4 Serving Stale Responses
		// RFC 9111 §4.3.3 Handling Validation Responses (5xx errors)
		SetAgeHeader(ctx.Stored.Response, r.clock, ctx.Freshness.Age)
		CacheStatusStale.ApplyTo(ctx.Stored.Response.Header)
		return ctx.Stored.Response, nil
	default:
		if err != nil {
			return nil, err
		}
		// RFC 9111 §4.3.3 Handling Validation Responses (full response)
		// RFC 9111 §3.2 Storing Responses
		ccResp := ParseCCResponseDirectives(resp.Header)
		if r.ce.CanStoreResponse(resp, ctx.CCReq, ccResp) {
			_ = r.rs.StoreResponse(resp, ctx.CacheKey, ctx.Start, ctx.End)
			CacheStatusMiss.ApplyTo(resp.Header)
			return resp, nil
		} else {
			r.ci.InvalidateCache(req.URL, resp.Header, ctx.CacheKey)
			CacheStatusBypass.ApplyTo(resp.Header)
			return resp, nil
		}
	}
}
