package internal

import (
	"net/http"
	"time"
)

type ValidationResponseHandler interface {
	HandleValidationResponse(
		ctx RevalidationContext,
		req *http.Request,
		resp *http.Response,
		err error,
	) (*http.Response, error)
}

type RevalidationContext struct {
	URLKey     string
	Start, End time.Time
	CCReq      CCRequestDirectives
	Stored     *Response
	Refs       ResponseRefs
	RefIndex   int
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
		updateStoredHeaders(ctx.Stored.Data, resp)
		CacheStatusRevalidated.ApplyTo(ctx.Stored.Data.Header)
		return ctx.Stored.Data, nil
	case (err != nil || isStaleErrorAllowed(resp.StatusCode)) &&
		req.Method == http.MethodGet &&
		r.siep.CanStaleOnError(ctx.Freshness, ParseCCResponseDirectives(resp.Header)):
		// RFC 9111 §4.2.4 Serving Stale Responses
		// RFC 9111 §4.3.3 Handling Validation Responses (5xx errors)
		SetAgeHeader(ctx.Stored.Data, r.clock, ctx.Freshness.Age)
		CacheStatusStale.ApplyTo(ctx.Stored.Data.Header)
		return ctx.Stored.Data, nil
	default:
		if err != nil {
			return nil, err
		}
		// RFC 9111 §4.3.3 Handling Validation Responses (full response)
		// RFC 9111 §3.2 Storing Responses
		ccResp := ParseCCResponseDirectives(resp.Header)
		if r.ce.CanStoreResponse(resp, ctx.CCReq, ccResp) {
			_ = r.rs.StoreResponse(
				req,
				resp,
				ctx.URLKey,
				ctx.Refs,
				ctx.Start,
				ctx.End,
				ctx.RefIndex,
			)
			CacheStatusMiss.ApplyTo(resp.Header)
		} else {
			if IsUnsafeMethod(req) && IsNonErrorStatus(resp.StatusCode) {
				// RFC 9111 §4.4 Invalidation of Cache Entries
				r.ci.InvalidateCache(req.URL, resp.Header, ctx.Refs, ctx.URLKey)
			}
			CacheStatusBypass.ApplyTo(resp.Header)
		}
		return resp, nil
	}
}
