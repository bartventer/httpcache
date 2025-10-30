// Copyright (c) 2025 Bart Venter <bartventer@proton.me>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package fscache

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/bartventer/httpcache/internal/testutil"
)

func Test_Issue16_LongURLFragmentation(t *testing.T) {
	t.Attr("GOOS", runtime.GOOS)
	t.Attr("GOARCH", runtime.GOARCH)
	t.Attr("GOVERSION", runtime.Version())

	tempDir := t.TempDir()
	cache, err := Open("test-fragmentation", WithBaseDir(tempDir))
	testutil.RequireNoError(t, err)

	tests := []struct {
		name     string
		urlLen   int
		expectOK bool
	}{
		{"normal URL", 100, true},
		{"long URL (1KB)", 1024, true},
		{"very long URL (4KB)", 4096, true},
		{"extremely long URL (10KB)", 10240, true},
		{
			// While Go's stdlib handles long paths via \\?\ prefix on Windows,
			// extremely deep directory hierarchies (2844+ levels) may still hit
			// practical filesystem or OS limits beyond just path length.
			// See fixLongPath logic in os/path_windows.go (https://cs.opensource.google/go/go/+/refs/tags/go1.25.3:src/os/path_windows.go;l=100;drc=79b809afb325ae266497e21597f126a3e98a1ef7)
			"massive URL (100KB)",
			102400,
			runtime.GOOS != "windows",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Generate URL of specified length
			url := "https://example.com/" + strings.Repeat(
				"x",
				tt.urlLen-len("https://example.com/"),
			)

			// Get fragmentation details
			filename := cache.fn.FileName(url)
			depth := strings.Count(filename, string(os.PathSeparator))
			t.Attr("URLLength", strconv.Itoa(len(url)))
			t.Attr("PathLength", strconv.Itoa(len(filename)))
			t.Attr("DirectoryDepth", strconv.Itoa(depth))

			// Test round-trip: Set -> Get -> Delete
			data := []byte("test data")
			err := cache.Set(url, data)

			if tt.expectOK {
				testutil.RequireNoError(t, err)

				retrieved, getErr := cache.Get(url)
				testutil.RequireNoError(t, getErr)
				testutil.AssertEqual(t, string(data), string(retrieved))

				setErr := cache.Delete(url)
				testutil.RequireNoError(t, setErr)

				t.Logf("Successfully handled %d byte URL", len(url))
			} else if err == nil {
				t.Errorf("Expected error for %d byte URL, but got none", len(url))
			}
		})
	}
}

func Benchmark_Issue16_LongURLs(b *testing.B) {
	tempDir := b.TempDir()
	cache, err := Open("bench-long-urls", WithBaseDir(tempDir))
	if err != nil {
		b.Fatal(err)
	}

	urlLengths := []int{100, 1000, 10000, 50000}

	for _, length := range urlLengths {
		b.Run(fmt.Sprintf("url_length_%d", length), func(b *testing.B) {
			url := "https://example.com/" + strings.Repeat("x", length-len("https://example.com/"))
			data := []byte("benchmark data")

			b.ResetTimer()
			for i := 0; b.Loop(); i++ {
				key := fmt.Sprintf("%s-%d", url, i)

				err := cache.Set(key, data)
				if err != nil {
					b.Fatal(err)
				}

				_, err = cache.Get(key)
				if err != nil {
					b.Fatal(err)
				}

				err = cache.Delete(key)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
