package internal

import (
	"maps"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/bartventer/httpcache/internal/testutil"
)

func TestParseCCRequestDirectives_AllDirectives(t *testing.T) {
	header := http.Header{
		"Cache-Control": []string{
			`no-cache="foo,bar", max-age=1800, min-fresh=60, max-stale=120, no-store, only-if-cached, stale-if-error=90`,
		},
	}
	got := ParseCCRequestDirectives(header)

	want := CCRequestDirectives{
		"no-cache":  `"foo,bar"`,
		"max-age":   "1800",
		"min-fresh": "60",
		"max-stale": "120",
		"no-store":  "",
		// "no-transform":   "", // no impact on this implementation
		"only-if-cached": "",
		"stale-if-error": "90",
	}
	t.Run("map equality", func(t *testing.T) {
		testutil.AssertTrue(t, maps.Equal(want, got))
	})

	t.Run("MaxAge", func(t *testing.T) {
		maxAge, ok := got.MaxAge()
		testutil.RequireTrue(t, ok)
		testutil.AssertEqual(t, 1800*time.Second, maxAge)
	})

	t.Run("MinFresh", func(t *testing.T) {
		minFresh, ok := got.MinFresh()
		testutil.RequireTrue(t, ok)
		testutil.AssertEqual(t, 60*time.Second, minFresh)
	})

	t.Run("MaxStale", func(t *testing.T) {
		maxStale, ok := got.MaxStale()
		testutil.RequireTrue(t, ok)
		testutil.AssertEqual(t, RawDeltaSeconds("120"), maxStale)
	})

	t.Run("NoCache", func(t *testing.T) {
		testutil.AssertTrue(t, got.NoCache())
	})

	t.Run("NoStore", func(t *testing.T) {
		testutil.AssertTrue(t, got.NoStore())
	})

	t.Run("OnlyIfCached", func(t *testing.T) {
		testutil.AssertTrue(t, got.OnlyIfCached())
	})

	t.Run("StaleIfError", func(t *testing.T) {
		sie, ok := got.StaleIfError()
		testutil.RequireTrue(t, ok)
		testutil.AssertEqual(t, 90*time.Second, sie)
	})

	t.Run("NoCache (quoted CSV)", func(t *testing.T) {
		raw := got["no-cache"]
		noCacheSeq, valid := RawCSVSeq(ParseQuotedString(raw)).Value()
		testutil.RequireTrue(t, valid)
		expectedNoCache := []string{"foo", "bar"}
		var gotNoCache []string
		for v := range noCacheSeq {
			gotNoCache = append(gotNoCache, v)
		}
		testutil.AssertTrue(t, slices.Equal(expectedNoCache, gotNoCache))
	})
}

func TestParseCCResponseDirectives_AllDirectives(t *testing.T) {
	header := http.Header{
		"Cache-Control": []string{
			`no-cache="foo,bar", max-age=3600, must-revalidate, must-understand, no-store, public, stale-if-error=120, stale-while-revalidate=60`,
		},
	}
	got := ParseCCResponseDirectives(header)

	want := CCResponseDirectives{
		"no-cache":        `"foo,bar"`,
		"max-age":         "3600",
		"must-revalidate": "",
		"must-understand": "",
		"no-store":        "",
		// "no-transform":           "", // no impact on this implementation
		"public":                 "",
		"stale-if-error":         "120",
		"stale-while-revalidate": "60",
	}
	t.Run("map equality", func(t *testing.T) {
		testutil.AssertTrue(t, maps.Equal(want, got))
	})

	t.Run("MaxAge", func(t *testing.T) {
		maxAge, ok := got.MaxAge()
		testutil.RequireTrue(t, ok)
		testutil.AssertEqual(t, 3600*time.Second, maxAge)
	})

	t.Run("MustRevalidate", func(t *testing.T) {
		testutil.AssertTrue(t, got.MustRevalidate())
	})

	t.Run("MustUnderstand", func(t *testing.T) {
		testutil.AssertTrue(t, got.MustUnderstand())
	})

	t.Run("NoStore", func(t *testing.T) {
		testutil.AssertTrue(t, got.NoStore())
	})

	t.Run("Public", func(t *testing.T) {
		testutil.AssertTrue(t, got.Public())
	})

	t.Run("StaleIfError", func(t *testing.T) {
		sie, ok := got.StaleIfError()
		testutil.RequireTrue(t, ok)
		testutil.AssertEqual(t, 120*time.Second, sie)
	})

	t.Run("StaleWhileRevalidate", func(t *testing.T) {
		swr, ok := got.StaleWhileRevalidate()
		testutil.RequireTrue(t, ok)
		testutil.AssertEqual(t, 60*time.Second, swr)
	})

	t.Run("NoCache (quoted CSV)", func(t *testing.T) {
		noCacheRaw, present := got.NoCache()
		testutil.AssertTrue(t, present)
		noCacheSeq, valid := noCacheRaw.Value()
		testutil.RequireTrue(t, valid)
		expectedNoCache := []string{"foo", "bar"}
		var gotNoCache []string
		for v := range noCacheSeq {
			gotNoCache = append(gotNoCache, v)
		}
		testutil.AssertTrue(t, slices.Equal(expectedNoCache, gotNoCache))
	})
}
