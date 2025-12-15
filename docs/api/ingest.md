# Ingest API (Telemetry Demo)

**Status:** Draft contract (dogfood example).

## Endpoint

`POST /ingest`

## Request

### Headers
- `Content-Type: application/json`
- Optional: `X-Request-Id: <string>`

### Body

```json
{
  "source": "web|worker|cli",
  "event": "string",
  "timestamp": "RFC3339 string",
  "payload": {}
}
```

Notes:
- `payload` is an arbitrary JSON object; consumers should treat it as opaque.

## Responses

### 202 Accepted

Returned when the event is accepted for processing and persisted.

```json
{
  "status": "accepted",
  "id": "evt_123"
}
```

### 400 Bad Request

Returned when request JSON is invalid or required fields are missing.

```json
{
  "error": "invalid_request",
  "message": "missing field: event"
}
```

### 413 Payload Too Large

Returned when request exceeds server limits.

```json
{
  "error": "payload_too_large",
  "message": "max_size_bytes=1048576"
}
```

### 500 Internal Server Error

Returned for unexpected failures.

```json
{
  "error": "internal",
  "message": "unexpected error"
}
```

## Sample cURL

```bash
curl -s -X POST http://localhost:8080/ingest \
  -H 'Content-Type: application/json' \
  -d '{"source":"cli","event":"demo.ingest","timestamp":"2025-01-01T00:00:00Z","payload":{"ok":true}}'
```

