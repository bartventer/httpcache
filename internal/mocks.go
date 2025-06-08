package internal

import (
	"net/http"
	"net/url"
	"time"
)

var _ Cache = (*MockCache)(nil)

type MockCache struct {
	GetFunc    func(key string) ([]byte, error)
	SetFunc    func(key string, entry []byte) error
	DeleteFunc func(key string) error
}

func (m *MockCache) Get(key string) ([]byte, error) {
	return m.GetFunc(key)
}

func (m *MockCache) Set(key string, entry []byte) error {
	return m.SetFunc(key, entry)
}

func (m *MockCache) Delete(key string) error {
	return m.DeleteFunc(key)
}

var _ ResponseCache = (*MockResponseCache)(nil)

type MockResponseCache struct {
	GetFunc        func(key string, req *http.Request) (*Entry, error)
	SetFunc        func(key string, entry *Entry) error
	DeleteFunc     func(key string) error
	GetHeadersFunc func(key string) (HeaderEntries, error)
	SetHeadersFunc func(key string, headers HeaderEntries) error
}

// GetHeaders implements ResponseCache.
func (m *MockResponseCache) GetHeaders(key string) (HeaderEntries, error) {
	return m.GetHeadersFunc(key)
}

// SetHeaders implements ResponseCache.
func (m *MockResponseCache) SetHeaders(key string, headers HeaderEntries) error {
	return m.SetHeadersFunc(key, headers)
}

func (m *MockResponseCache) Get(key string, req *http.Request) (*Entry, error) {
	return m.GetFunc(key, req)
}
func (m *MockResponseCache) Set(key string, entry *Entry) error {
	return m.SetFunc(key, entry)
}
func (m *MockResponseCache) Delete(key string) error {
	return m.DeleteFunc(key)
}

var _ RequestMethodChecker = (*MockRequestMethodChecker)(nil)

type MockRequestMethodChecker struct {
	IsRequestMethodUnderstoodFunc func(req *http.Request) bool
}

func (m *MockRequestMethodChecker) IsRequestMethodUnderstood(req *http.Request) bool {
	return m.IsRequestMethodUnderstoodFunc(req)
}

var _ VaryMatcher = (*MockVaryMatcher)(nil)

type MockVaryMatcher struct {
	VaryHeadersMatchFunc func(cachedHdrs HeaderEntries, reqHdr http.Header) (int, bool)
}

func (m *MockVaryMatcher) VaryHeadersMatch(
	cachedHdrs HeaderEntries,
	reqHdr http.Header,
) (int, bool) {
	return m.VaryHeadersMatchFunc(cachedHdrs, reqHdr)
}

var _ URLKeyer = (*MockCacheKeyer)(nil)

type MockCacheKeyer struct {
	CacheKeyFunc func(u *url.URL) string
}

func (m *MockCacheKeyer) URLKey(u *url.URL) string {
	return m.CacheKeyFunc(u)
}

var _ StaleIfErrorPolicy = (*MockStaleIfErrorPolicy)(nil)

type MockStaleIfErrorPolicy struct {
	CanStaleOnErrorFunc func(freshness *Freshness, sies ...StaleIfErrorer) bool
}

func (m *MockStaleIfErrorPolicy) CanStaleOnError(
	freshness *Freshness,
	sies ...StaleIfErrorer,
) bool {
	return m.CanStaleOnErrorFunc(freshness, sies...)
}

var _ FreshnessCalculator = (*MockFreshnessCalculator)(nil)

type MockFreshnessCalculator struct {
	CalculateFreshnessFunc func(resp *http.Response, reqCC CCRequestDirectives, resCC CCResponseDirectives) *Freshness
}

func (m *MockFreshnessCalculator) CalculateFreshness(
	resp *Entry,
	reqCC CCRequestDirectives,
	resCC CCResponseDirectives,
) *Freshness {
	return m.CalculateFreshnessFunc(resp.Response, reqCC, resCC)
}

var _ Clock = (*MockClock)(nil)

type MockClock struct {
	NowResult   time.Time
	SinceResult time.Duration
}

func (m *MockClock) Now() time.Time                  { return m.NowResult }
func (m *MockClock) Since(t time.Time) time.Duration { return m.SinceResult }

var _ http.RoundTripper = (*MockRoundTripper)(nil)

type MockRoundTripper struct {
	RoundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.RoundTripFunc(req)
}

var _ ResponseStorer = (*MockResponseStorer)(nil)

type MockResponseStorer struct {
	StoreResponseFunc func(resp *http.Response, key string, headers HeaderEntries, reqTime, respTime time.Time) error
}

func (m *MockResponseStorer) StoreResponse(
	resp *http.Response,
	key string,
	headers HeaderEntries,
	reqTime, respTime time.Time,
) error {
	return m.StoreResponseFunc(resp, key, headers, reqTime, respTime)
}

var _ CacheInvalidator = (*MockCacheInvalidator)(nil)

type MockCacheInvalidator struct {
	InvalidateCacheFunc func(reqURL *url.URL, respHeader http.Header, headers HeaderEntries, key string)
}

func (m *MockCacheInvalidator) InvalidateCache(
	reqURL *url.URL,
	respHeader http.Header,
	headers HeaderEntries,
	key string,
) {
	m.InvalidateCacheFunc(reqURL, respHeader, headers, key)
}

var _ ValidationResponseHandler = (*MockValidationResponseHandler)(nil)

type MockValidationResponseHandler struct {
	HandleValidationResponseFunc func(ctx RevalidationContext, req *http.Request, resp *http.Response, err error) (*http.Response, error)
}

func (m *MockValidationResponseHandler) HandleValidationResponse(
	ctx RevalidationContext,
	req *http.Request,
	resp *http.Response,
	err error,
) (*http.Response, error) {
	return m.HandleValidationResponseFunc(ctx, req, resp, err)
}

var _ VaryMatcher = (*MockVaryMatcher)(nil)
