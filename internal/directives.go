package internal

import (
	"iter"
	"maps"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

/*
Cache-Control Directives Supported

Freshness Directives (affect staleness calculation):
- max-age (request/response)
- expires (response)
- min-fresh (request)
- max-stale (request)
- s-maxage (ignored in private caches)

Policy Directives (affect use/revalidation):
- no-cache (request/response)
- must-revalidate (response)
- only-if-cached (request)

Storage Control:
- no-store (request/response)
- public (response)
- no-transform (request/response)
- must-understand (response)
- must-revalidate (response)

Ignored Directives (not applicable to private caches):
- proxy-revalidate (response)
- private (response)

Staleness Tolerance Extensions (allow use of stale responses under certain conditions):
- stale-while-revalidate (response): may serve stale while background revalidation for N seconds beyond freshness lifetime
- stale-if-error (response): may serve stale if origin returns error (e.g., 5xx) for N seconds beyond freshness lifetime

Design Note:
- Freshness calculation is based strictly on age/lifetime and freshness directives.
- Policy directives (e.g., no-cache, must-revalidate, stale-while-revalidate, stale-if-error) are enforced outside of freshness calculation.
- This separation aligns with RFC 9111 and RFC 5861 recommendations.
*/

// RawTime is a string that represents a time in HTTP date format.
type RawTime string

// Value returns the time and a boolean indicating whether the result is valid.
func (r RawTime) Value() (t time.Time, valid bool) {
	if len(r) == 0 {
		return
	}
	parsedTime, err := http.ParseTime(string(r))
	if err != nil {
		return
	}
	return parsedTime, true
}

// RawDeltaSeconds is a string that represents a delta time in seconds,
// as defined in §1.2.2 of RFC 9111.
//
// This implementation supports values up to the maximum range of int64
// (9223372036854775807 seconds). Values exceeding 2147483648 (2^31) are
// valid and will not be capped, as allowed by the RFC, which permits
// using the greatest positive integer the implementation can represent.
type RawDeltaSeconds string

func (r RawDeltaSeconds) Value() (dur time.Duration, valid bool) {
	if len(r) == 0 || r[0] == '-' {
		return
	}
	seconds, err := strconv.ParseInt(string(r), 10, 64)
	if err != nil {
		return
	}

	return time.Duration(seconds) * time.Second, true
}

// TrimmedCSVSeq returns an iterator over the raw comma-separated string.
// It yields each part of the string, trimmed of whitespace, and does not split inside quoted strings.
func TrimmedCSVSeq(s string) iter.Seq[string] {
	return func(yield func(string) bool) {
		var part strings.Builder
		inQuotes := false
		escape := false
		for i := range len(s) {
			c := s[i]
			switch {
			case escape:
				part.WriteByte(c)
				escape = false
			case c == '\\':
				part.WriteByte(c)
				escape = true
			case c == '"':
				part.WriteByte(c)
				inQuotes = !inQuotes
			case c == ',' && !inQuotes:
				p := textproto.TrimString(part.String())
				if len(p) > 0 {
					if !yield(p) {
						return
					}
				}
				part.Reset()
			default:
				part.WriteByte(c)
			}
		}
		if part.Len() > 0 {
			p := textproto.TrimString(part.String())
			if len(p) > 0 {
				_ = yield(p)
			}
		}
	}
}

// RawCSVSeq is a string that represents a sequence of comma-separated values.
type RawCSVSeq string

// Value returns an iterator over the raw comma-separated string and a boolean indicating
// whether the result is valid.
func (s RawCSVSeq) Value() (seq iter.Seq[string], valid bool) {
	if len(s) == 0 {
		return
	}
	return TrimmedCSVSeq(string(s)), true
}

// directivesSeq2 returns an iterator over all key-value pairs in a string of
// cache directives (as specified in 9111, §5.2.1 and 5.2.2). The
// iterator yields the key (token) and value (argument) of each directive.
//
// It guarentees that the key is always non-empty, and if a value is not
// present, it yields an empty string as the value.
func directivesSeq2(s string) iter.Seq2[string, string] {
	return func(yield func(string, string) bool) {
		for part := range TrimmedCSVSeq(s) {
			key, value, found := strings.Cut(part, "=")
			if !found {
				key = textproto.TrimString(part)
				value = ""
			} else {
				// value = textproto.TrimString(ParseQuotedString(value))
				value = textproto.TrimString(value)
			}
			if len(key) == 0 {
				continue
			}
			if !yield(key, value) {
				return
			}
		}
	}
}

// parseDirectives parses a string of cache directives and returns a map
// where the keys are the directive names and the values are the arguments.
func parseDirectives(s string) map[string]string {
	return maps.Collect(directivesSeq2(s))
}

func hasToken(d map[string]string, token string) bool {
	_, ok := d[token]
	return ok
}

func getDurationDirective(d map[string]string, token string) (dur time.Duration, valid bool) {
	if v, ok := d[token]; ok {
		return RawDeltaSeconds(v).Value()
	}
	return
}

// CCRequestDirectives is a map of request directives from the Cache-Control
// header field. The keys are the directive names and the values are the arguments.
type CCRequestDirectives map[string]string

func ParseCCRequestDirectives(header http.Header) CCRequestDirectives {
	value := header.Get("Cache-Control")
	if value == "" {
		return nil
	}
	return parseDirectives(value)
}

// MaxAge parses the "max-age" request directive as defined in RFC 9111, §5.2.1.1.
// It indicates the client's maximum acceptable age for a cached response.
func (d CCRequestDirectives) MaxAge() (dur time.Duration, valid bool) {
	return getDurationDirective(d, "max-age")
}

// MaxStale parses the "max-stale" request directive as defined in RFC 9111, §5.2.1.2.
// Indicates the client's maximum acceptable staleness of a cached response.
func (d CCRequestDirectives) MaxStale() (dur RawDeltaSeconds, valid bool) {
	if v, ok := d["max-stale"]; ok {
		return RawDeltaSeconds(v), true
	}
	return
}

// MinFresh parses the "min-fresh" request directive as defined in RFC 9111, §5.2.1.3.
// It indicates the minimum time a cached response must remain fresh before it can be served.
func (d CCRequestDirectives) MinFresh() (dur time.Duration, valid bool) {
	return getDurationDirective(d, "min-fresh")
}

// NoCache reports the presence of the "no-cache" request directive as defined in RFC 9111, §5.2.1.4.
// It indicates that the request must be validated with the origin server before being served from cache.
func (d CCRequestDirectives) NoCache() bool {
	return hasToken(d, "no-cache")
}

// NoStore reports the presence of the "no-store" request directive as defined in RFC 9111, §5.2.1.5.
// It indicates that the request must not be stored by any cache.
func (d CCRequestDirectives) NoStore() bool {
	return hasToken(d, "no-store")
}

// It indicates that the client does not want any transformation of the response content,.
func (d CCRequestDirectives) NoTransform() bool {
	return hasToken(d, "no-transform")
}

// OnlyIfCached reports the presence of the "only-if-cached" request directive as defined in RFC 9111, §5.2.1.7.
// It indicates that the client only wants a response from the cache and does not want to contact the origin server.
// If the response is not in the cache, the server should return a 504 (Gateway Timeout) status code.
func (d CCRequestDirectives) OnlyIfCached() bool {
	return hasToken(d, "only-if-cached")
}

// StaleIfError parses the "stale-if-error" request directive (extension) as defined in RFC 5861, §4.
// It indicates that the client is willing to accept a stale response if the origin server is unavailable or returns an error.
func (d CCRequestDirectives) StaleIfError() (dur time.Duration, valid bool) {
	return getDurationDirective(d, "stale-if-error")
}

// CCResponseDirectives is a map of response directives from the Cache-Control
// header field. The keys are the directive names and the values are the arguments.
//
// The following directives per RFC 9111, §5.2.2 are not applicable to private caches:
//   - "proxy-revalidate" (§5.2.2.8)
//   - "public" (§5.2.2.9)
//   - "s-maxage" (§5.2.2.10)
//
// Additionally, the following extension directives are supported:
//   - "stale-if-error" (RFC 5861, §4)
type CCResponseDirectives map[string]string

func ParseCCResponseDirectives(header http.Header) CCResponseDirectives {
	value := header.Get("Cache-Control")
	if value == "" {
		return nil
	}
	return parseDirectives(value)
}

// MaxAge parses the "max-age" response directive as defined in RFC 9111, §5.2.2.1.
// It indicates the maximum time a response can be cached before it must be revalidated.
func (d CCResponseDirectives) MaxAge() (dur time.Duration, valid bool) {
	return getDurationDirective(d, "max-age")
}

// MaxAgePresent reports the presence of the "max-age" response directive as defined in RFC 9111, §5.2.2.1.
// It indicates that the response has a valid "max-age" directive, regardless of its value.
func (d CCResponseDirectives) MaxAgePresent() bool {
	return hasToken(d, "max-age")
}

// MustRevalidate reports the presence of the "must-revalidate" response directive as defined in RFC 9111, §5.2.2.2.
func (d CCResponseDirectives) MustRevalidate() bool {
	return hasToken(d, "must-revalidate")
}

// MustUnderstand reports the presence of the "must-understand" response directive as defined in RFC 9111, §5.2.2.3.
func (d CCResponseDirectives) MustUnderstand() bool {
	return hasToken(d, "must-understand")
}

// NoCache parses the "no-cache" response directive as defined in RFC 9111, §5.2.2.4.
// If the directive is present, it returns the raw comma-separated values.
func (d CCResponseDirectives) NoCache() (fields RawCSVSeq, present bool) {
	v, ok := d["no-cache"]
	if !ok {
		return
	}
	return RawCSVSeq(ParseQuotedString(v)), true
}

// NoStore reports the presence of the "no-store" response directive as defined in RFC 9111, §5.2.2.5.
func (d CCResponseDirectives) NoStore() bool {
	return hasToken(d, "no-store")
}

// NoTransform reports the presence of the "no-transform" response directive as defined in RFC 9111, §5.2.2.6.
func (d CCResponseDirectives) NoTransform() bool {
	return hasToken(d, "no-transform")
}

// Public reports the presence of the "public" response directive as defined in RFC 9111, §5.2.2.9.
func (d CCResponseDirectives) Public() bool {
	return hasToken(d, "public")
}

// StaleIfError parses the "stale-if-error" response directive (extension) as defined in RFC 5861, §4.
// It indicates that a cache can serve a stale response if the origin server is unavailable or returns an error.
func (d CCResponseDirectives) StaleIfError() (dur time.Duration, valid bool) {
	return getDurationDirective(d, "stale-if-error")
}

// StaleWhileRevalidate parses the "stale-while-revalidate" response directive (extension) as defined in RFC 5861, §3.
// It indicates that a cache can serve a stale response while it revalidates the response in the background.
func (d CCResponseDirectives) StaleWhileRevalidate() (dur time.Duration, valid bool) {
	return getDurationDirective(d, "stale-while-revalidate")
}
