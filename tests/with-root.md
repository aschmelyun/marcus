---
root: https://httpbin.org
headers:
  Accept: application/json
---

# Tests with Root URL

These tests use relative paths with a base URL defined in frontmatter.

## Test 1: GET with relative path

GET /get

Asserts:
- Status is 200
- Body contains `url`

## Test 2: POST with relative path

POST /post
- Content-Type: application/json

```json
{
  "name": "marcus",
  "type": "api-tester"
}
```

Asserts:
- Status is 200
- Field `json.name` equals `marcus`

## Test 3: Headers endpoint with path

GET /headers

Asserts:
- Status is 200
- Field `headers.Accept` equals `application/json`

## Test 4: Absolute URL still works

GET https://httpbin.org/user-agent

Asserts:
- Status is 200
- Body contains `user-agent`
