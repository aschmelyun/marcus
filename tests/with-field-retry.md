---
root: https://httpbin.org
---

# Tests with Field-based Wait Functionality

These tests demonstrate waiting for a specific field value in the response body.

## Test 1: Wait for field value (immediate match)

This endpoint returns a JSON response with predictable fields.
The `url` field will match immediately, so no retries are needed.

GET /get
- Wait until field `url` equals `https://httpbin.org/get`
- Retry 3 times every 100ms

Asserts:
- Status is 200
- Body contains `url`

## Test 2: Wait for nested field value

The /get endpoint returns headers in a nested object.

GET /get
- Accept: application/json
- Wait until field `headers.Accept` equals `application/json`
- Retry 3 times every 100ms

Asserts:
- Status is 200
- Field `headers.Accept` equals `application/json`

## Test 3: Combined status and field wait

Wait for both a status code and a field value.

GET /get
- Wait until status is 200
- Wait until field `url` equals `https://httpbin.org/get`
- Retry 5 times every 200ms

Asserts:
- Status is 200
- Body contains `headers`

## Test 4: Wait for field in POST response

POST /post
- Content-Type: application/json
- Wait until field `json.status` equals `ok`
- Retry 3 times every 100ms

```json
{"status": "ok", "message": "test"}
```

Asserts:
- Status is 200
- Field `json.status` equals `ok`
- Field `json.message` equals `test`
