# httpcache

[![Go Reference](https://pkg.go.dev/badge/github.com/bartventer/httpcache.svg)](https://pkg.go.dev/github.com/bartventer/httpcache)
[![Go Report Card](https://goreportcard.com/badge/github.com/bartventer/httpcache)](https://goreportcard.com/report/github.com/bartventer/httpcache)
[![Test](https://github.com/bartventer/httpcache/actions/workflows/default.yml/badge.svg)](https://github.com/bartventer/httpcache/actions/workflows/default.yml)
[![codecov](https://codecov.io/github/bartventer/httpcache/graph/badge.svg?token=pnpoA3t4EE)](https://codecov.io/github/bartventer/httpcache)

*httpcache* is a Go package that provides a standards-compliant [http.RoundTripper](https://pkg.go.dev/net/http#RoundTripper) for transparent HTTP response caching, following [RFC 9111 (HTTP Caching)](https://www.rfc-editor.org/rfc/rfc9111).

> **Note:** This package is intended for use as a **private (client-side) cache**. It is not a shared or proxy cache.

## Features

- **Plug-and-Play**: Drop-in replacement for [`http.RoundTripper`](https://pkg.go.dev/net/http#RoundTripper) with no additional configuration required.
- **RFC 9111 Compliance**: Handles validation, expiration, and revalidation.
- **Cache Control**: Supports all relevant HTTP cache control directives, as well as extensions like [`stale-while-revalidate`](https://www.rfc-editor.org/rfc/rfc5861#section-3) and [`stale-if-error`](https://www.rfc-editor.org/rfc/rfc5861#section-4).
- **Cache Backends**: Built-in support for file system and memory caches, with the ability to implement custom backends.
- **Extensible**: Options for logging, transport and timeouts.
- **Debuggable**: Adds a cache status header to every response.
- **Zero Dependencies**: No external dependencies, pure Go implementation.

![Made with VHS](https://vhs.charm.sh/vhs-3WOBtYTZzzXggFGYRudHTV.gif)

*Refer to [_examples/app](_examples/app/app.go) for the source code.*

## Quick Start

```go
package main

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/bartventer/httpcache"
    // Register the file system cache backend
	_ "github.com/bartventer/httpcache/store/fscache" 
)

func main() {
    // Example DSN for the file system cache backend
	dsn := "fscache://?appname=myapp" 
	client := &http.Client{
		Transport: httpcache.NewTransport(
			dsn,
			httpcache.WithSWRTimeout(10*time.Second),
			httpcache.WithLogger(slog.Default()),
		),
	}
    // ... Use the client as usual
}
```

> To use a cache backend, specify it with a DSN string (e.g., `"fscache://"` or `"memcache://"`).  **You must import the backend package (often with a blank import) to register it.**


## Cache Backends

| Backend                                                                         | DSN Example                | Description                                          |
| ------------------------------------------------------------------------------- | -------------------------- | ---------------------------------------------------- |
| [`fscache`](https://pkg.go.dev/github.com/bartventer/httpcache/store/fscache)   | `fscache://?appname=myapp` | Built-in file system cache, stores responses on disk |
| [`memcache`](https://pkg.go.dev/github.com/bartventer/httpcache/store/memcache) | `memcache://`              | Built-in memory cache, stores responses in memory    |

Consult the documentation for each backend for specific configuration options and usage details.

### Custom Cache Backends

To implement a custom cache backend, create a type that satisfies the `httpcache.Cache` interface. This interface requires methods for storing, retrieving, and deleting cached responses. You can then register your backend by importing it in your application. See the [fscache](https://pkg.go.dev/github.com/bartventer/httpcache/store/fscache) for an example implementation.

## Options

| Option                             | Description                            | Default Value                   |
| ---------------------------------- | -------------------------------------- | ------------------------------- |
| `WithTransport(http.RoundTripper)` | Set the underlying transport           | `http.DefaultTransport`         |
| `WithSWRTimeout(time.Duration)`    | Set the stale-while-revalidate timeout | `5s`                            |
| `WithLogger(*slog.Logger)`         | Set a logger for debug output          | `slog.New(slog.DiscardHandler)` |

## Supported Cache-Control Directives

| Directive                | Request | Response | Description                                             |
| ------------------------ | ------- | -------- | ------------------------------------------------------- |
| `max-age`                | ✔       | ✔        | Maximum age for cache freshness                         |
| `min-fresh`              | ✔       |          | Minimum freshness required                              |
| `max-stale`              | ✔       |          | Accept response stale by up to N seconds                |
| `no-cache`               | ✔       | ✔        | Must revalidate with origin before using                |
| `no-store`               | ✔       | ✔        | Do not store in any cache                               |
| `only-if-cached`         | ✔       |          | Only serve from cache, never contact origin             |
| `must-revalidate`        |         | ✔        | Must revalidate once stale                              |
| `must-understand`        |         | ✔        | Require cache to understand directive                   |
| `public`                 |         | ✔        | Response may be cached, even if normally non-cacheable  |
| `stale-while-revalidate` |         | ✔        | Serve stale while revalidating in background (RFC 5861) |
| `stale-if-error`         | ✔       | ✔        | Serve stale if origin returns error (RFC 5861)          |

> **Note:** The `private`, `proxy-revalidate`, and `s-maxage` directives bear no relevance in a private client-side cache and are ignored.

## Cache Status Header

Every response includes a cache status header to indicate how the response was served. The header is named `X-Cache-Status` and can have the following values:

| Status        | Description             |
| ------------- | ----------------------- |
| `HIT`         | Served from cache       |
| `MISS`        | Fetched from origin     |
| `STALE`       | Served stale from cache |
| `REVALIDATED` | Revalidated with origin |
| `BYPASS`      | Cache bypassed          |

### Example

```http
X-Cache-Status: HIT
```

## Limitations

- **Range Requests & Partial Content:**
  This cache does **not** support HTTP range requests or partial/incomplete responses (e.g., status code 206, `Range`/`Content-Range` headers). All requests with a `Range` header are bypassed, and 206 responses are not cached. See [RFC 9111 §3.3-3.4](https://www.rfc-editor.org/rfc/rfc9111#section-3.3) for details.

## License

This project is licensed under the [Apache License 2.0](https://www.apache.org/licenses/LICENSE-2.0). See the [LICENSE](LICENSE) file for details.
