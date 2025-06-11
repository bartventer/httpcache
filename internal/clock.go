package internal

import (
	"net/http"
	"time"
)

type Clock interface {
	Now() time.Time
	Since(t time.Time) time.Duration
}

type clock struct{}

func NewClock() *clock { return &clock{} }

func (c clock) Since(t time.Time) time.Duration { return time.Since(t) }
func (c clock) Now() time.Time                  { return time.Now() }

// FixDateHeader sets the "Date" header to the current time in UTC if it is
// missing or empty, as per RFC 9110 §6.6.1, and reports whether it was changed.
//
// NOTE: This cache forwards all requests to the client, so it MUST set the
// "Date" header to the current time for responses that do not have it set.
func FixDateHeader(h http.Header, receivedAt time.Time) bool {
	if date, valid := RawTime(h.Get("Date")).Value(); !valid || date.IsZero() {
		h.Set("Date", receivedAt.UTC().Format(http.TimeFormat))
		return true
	}
	return false
}
