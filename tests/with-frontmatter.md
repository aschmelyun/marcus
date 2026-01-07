---
headers:
  Accept: application/json
  X-Custom-Header: test-value
---

# Tests with Default Headers

## Test 1: Verify default headers are sent

GET https://httpbin.org/headers

Asserts:
- Status is 200
- Field `headers.Accept` equals `application/json`
- Field `headers.X-Custom-Header` equals `test-value`

## Test 2: Override a default header

GET https://httpbin.org/headers
- Accept: text/plain

Asserts:
- Status is 200
- Field `headers.Accept` equals `text/plain`
- Field `headers.X-Custom-Header` equals `test-value`
