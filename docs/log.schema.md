# Log Schema for HTTP Cache

This document describes the structured log schema, in JSON format, used by the HTTP cache service. The schema is designed to provide detailed, machine-readable logs for cache operations, including hits, misses, stale responses, revalidations, bypasses, and errors.

| Field     | Type   | Description                                                       |
| --------- | ------ | ----------------------------------------------------------------- |
| timestamp | string | ISO8601 timestamp of the log entry                                |
| level     | string | Log level: `DEBUG`, `INFO`, `WARN`, `ERROR`                       |
| service   | string | Always `httpcache`                                                |
| event     | string | Event type: `HIT`, `MISS`, etc.                                   |
| msg       | string | Human-readable summary                                            |
| trace_id  | string | Trace/correlation ID (if provided)                                |
| error     | string | Error message (only for error logs)                               |
| request   | object | HTTP request context (see [below](#request-object))               |
| cache     | object | Cache context (see [below](#cache-object))                        |
| misc      | object | Additional context (optional, see [below](#misc-object-optional)) |

## `request` object

| Field  | Type   | Description      |
| ------ | ------ | ---------------- |
| method | string | HTTP method      |
| url    | string | Full request URL |
| host   | string | Host header      |

## `cache` object

| Field   | Type   | Description         |
| ------- | ------ | ------------------- |
| status  | string | HIT, MISS, STALE... |
| url_key | string | Cache key           |

## `misc` object (optional)

| Field       | Type   | Description                                                             |
| ----------- | ------ | ----------------------------------------------------------------------- |
| cc_request  | object | Cache-Control request directives (if present)                           |
| cc_response | object | Cache-Control response directives (if present)                          |
| stored      | object | Cached response details (if present)                                    |
| freshness   | object | Freshness details (if present, see [below](#freshness-object-optional)) |
| ref         | object | Reference details (if present)                                          |

## `freshness` object (optional)

| Field     | Type    | Description                                                    |
| --------- | ------- | -------------------------------------------------------------- |
| is_stale  | boolean | Whether the response is stale                                  |
| age       | object  | Age of the cached response (see [below](#age-object-optional)) |
| timestamp | string  | Timestamp of the age calculation (ISO8601)                     |

## `age` object (optional)

| Field     | Type   | Description                                |
| --------- | ------ | ------------------------------------------ |
| value     | number | Age in seconds                             |
| timestamp | string | Timestamp of the age calculation (ISO8601) |

### Example

```json
{
  "timestamp": "2025-07-01T14:20:00.000Z",
  "level": "INFO",
  "service": "httpcache",
  "event": "HIT",
  "msg": "Cache hit; served from cache.",
  "trace_id": "abc123def456",
  "request": {
    "method": "GET",
    "url": "https://api.example.com/data",
    "host": "api.example.com"
  },
  "cache": {
    "status": "HIT",
    "url_key": "https://api.example.com/data"
  },
  "misc": {
    "cc_request": {"max-age":"60"},
    "cc_response": {"max-age":"120", "stale-while-revalidate":"30"},
    "freshness": {
      "is_stale":false,
      "age": {
        "value":298741541,
        "timestamp":"2025-07-01T14:35:19.298742743Z"
      },
    }
  }
}
```
