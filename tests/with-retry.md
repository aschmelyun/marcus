---
root: https://httpbin.org
---

# Tests with Retry/Wait Functionality

These tests demonstrate waiting for a specific status code.

## Test 1: Immediate success (no retry needed)

This endpoint returns 200 immediately, so no retries are needed.

GET /status/200
- Wait for status: 200
- Retry-Max: 3
- Retry-Delay: 100ms

Asserts:
- Status is 200

## Test 2: Wait for status with custom settings

GET /get
- Wait for status: 200
- Retry-Delay: 500ms
- Retry-Max: 5

Asserts:
- Status is 200
- Body contains `url`

## Test 3: Normal request without retry (baseline)

GET /get

Asserts:
- Status is 200
- Body contains `headers`
