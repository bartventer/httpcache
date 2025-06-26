package internal

import (
	"net/http"
)

const (
	CacheStatusHeader       = "X-Httpcache-Status"
	CacheStatusHeaderLegacy = "X-Cache-Status" // Deprecated: use [CacheStatusHeader] instead
)

type CacheStatus struct {
	Value string
	// Value for compatibility with github.com/gregjones/httpcache:
	// 	"1" means served from cache (less specific "HIT")
	// 	"" means not served from cache (less specific "MISS")
	//
	// Deprecated: only used for compatibility with unmaintained (still widely
	// used) github.com/gregjones/httpcache; use Value instead.
	Legacy string
}

func (s CacheStatus) ApplyTo(header http.Header) {
	header.Set(CacheStatusHeader, s.Value)
	if s.Legacy != "" {
		header.Set(CacheStatusHeaderLegacy, s.Legacy)
	}
}

var (
	CacheStatusHit         = CacheStatus{"HIT", "1"}         // served from cache
	CacheStatusMiss        = CacheStatus{"MISS", ""}         // served from origin
	CacheStatusStale       = CacheStatus{"STALE", "1"}       // served from cache but stale
	CacheStatusRevalidated = CacheStatus{"REVALIDATED", "1"} // revalidated with origin server
	CacheStatusBypass      = CacheStatus{"BYPASS", ""}       // cache bypassed
)
