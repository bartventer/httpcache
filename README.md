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
- **Advanced directives**: Supports [`stale-while-revalidate`](https://www.rfc-editor.org/rfc/rfc5861), [`stale-if-error`](https://www.rfc-editor.org/rfc/rfc5861), and more.
- **Custom cache backends**: Bring your own cache implementation
- **Extensible**: Options for logging, transport and timeouts.
- **Debuggable**: Adds a cache status header to every response

![Made with VHS](https://vhs.charm.sh/vhs-3WOBtYTZzzXggFGYRudHTV.gif)

_Refer to [_examples/app](_examples/app/app.go) for the source code._

## Quick Start

```go
package main

import (
    "fmt"
    "net/http"

    "github.com/bartventer/httpcache"
)

func main() {
    myCache := /* your implementation of httpcache.Cache */
    client := &http.Client{
        Transport: httpcache.NewTransport(myCache)

    resp, err := client.Get("https://example.com/resource")
    if err != nil {
        fmt.Println("Error:", err)
        return
    }
    defer resp.Body.Close()
    fmt.Println(resp.Header.Get(httpcache.CacheStatusHeader)) // e.g. "HIT", "MISS"
}
```

## Cache Interface

The `httpcache.Cache` interface defines the methods required for a cache implementation:

```go
type Cache interface {
    Get(key string) ([]byte, error)
    Set(key string, value []byte) error
    Delete(key string) error
}
```

## Options

- `WithTransport(tr http.RoundTripper)`: Set the underlying transport (default: `http.DefaultTransport`).
- `WithSWRTimeout(timeout time.Duration)`: Set the stale-while-revalidate timeout (default: `5s`).
- `WithLogger(logger *slog.Logger)`: Set a logger (default: discard).

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

Every response includes a cache status header to indicate how the response was served:

| Status        | Description             |
| ------------- | ----------------------- |
| `HIT`         | Served from cache       |
| `MISS`        | Fetched from origin     |
| `STALE`       | Served stale from cache |
| `REVALIDATED` | Revalidated with origin |
| `BYPASS`      | Cache bypassed          |

## License

This project is licensed under the Apache License 2.0. See the [LICENSE](LICENSE) file for details.