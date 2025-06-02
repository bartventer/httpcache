package main

import (
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/bartventer/httpcache"
)

func main() {
	etag := "W/\"1234567890\""
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Duration(max(rand.IntN(800), 100)) * time.Millisecond)
		w.Header().Set("Cache-Control", "max-age=60")
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "Hello, client")
	}))
	defer ts.Close()

	url := ts.URL
	cache := NewDummyCache()
	client := &http.Client{
		Transport: httpcache.New(cache),
	}

	fmt.Println(strings.Repeat("=", 60))
	fmt.Println(" HTTP Cache Demo")
	fmt.Println(strings.Repeat("=", 60))

	var rows []string
	rows = append(rows, "Request\tStatus\tDuration\t")
	rows = append(rows, "-------\t------\t-------\t")

	for i := range 5 {
		fmt.Printf("\n\033[1;36m→ Sending request %d...\033[0m\n", i+1)
		start := time.Now()
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		if i > 0 {
			req.Header.Set("If-None-Match", etag)
		}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\033[1;31mRequest %d failed: %v\033[0m\n", i+1, err)
			continue
		}
		_, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		duration := time.Since(start).Truncate(time.Millisecond)
		cacheStatus := resp.Header.Get(httpcache.CacheStatusHeader)
		rows = append(rows, fmt.Sprintf(
			"%d\t%s\t%s\t%s\t",
			i+1,
			resp.Status,
			duration,
			colorize(cacheStatus),
		))
		fmt.Printf("\033[1;32m✓ Response received. Cache status: %s\033[0m\n", colorize(cacheStatus))
		fmt.Printf("Duration: \033[1;35m%s\033[0m\n", duration)
		fmt.Printf("Current cache entries: \033[1;34m%d\033[0m\n", len(cache.m))
		time.Sleep(1800 * time.Millisecond) // Slower for GIF/demo
	}

	fmt.Println()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', tabwriter.AlignRight)
	for _, row := range rows {
		fmt.Fprintln(w, row)
	}
	w.Flush()

	fmt.Println(strings.Repeat("-", 60))
	if len(cache.m) == 0 {
		fmt.Println("\033[1;31mCache should not be empty after requests\033[0m")
	} else {
		fmt.Printf("\033[1;34mCache contains %d entries\033[0m\n", len(cache.m))
		for k := range cache.m {
			fmt.Printf("  \033[1;36mCache key:\033[0m %q\n", k)
		}
	}
	fmt.Println(strings.Repeat("=", 60))
}

// DummyCache is a simple in-memory cache for demonstration.
type DummyCache struct {
	m map[string][]byte
}

func NewDummyCache() *DummyCache                         { return &DummyCache{m: make(map[string][]byte)} }
func (c *DummyCache) Get(key string) ([]byte, error)     { return c.m[key], nil }
func (c *DummyCache) Set(key string, entry []byte) error { c.m[key] = entry; return nil }
func (c *DummyCache) Delete(key string) error            { delete(c.m, key); return nil }

func colorize(status string) string {
	switch status {
	case "HIT":
		return "\033[1;32m🟢 HIT \033[0m"
	case "MISS":
		return "\033[1;31m🔴 MISS\033[0m"
	default:
		return "\033[1;33m🟡 " + status + "\033[0m"
	}
}
