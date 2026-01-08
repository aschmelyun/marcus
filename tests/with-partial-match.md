# Partial Match Tests

These tests demonstrate the `Body partially matches:` assertion, which allows you to check only specific fields in a JSON response while ignoring others (like timestamps, IDs, or dynamic values).

## Test 1: Partial match ignoring dynamic fields

The GET endpoint returns dynamic fields like `origin` (IP address) that change with each request. We only care about the static `url` field:

GET https://httpbin.org/get

Asserts:
- Status is 200
- Body partially matches:
```json
{
>>  "url": "https://httpbin.org/get",
    "origin": "this-will-change-every-time",
    "args": {}
}
```

## Test 2: Partial match on POST response

Response from POST requests often contain generated IDs and timestamps. We can verify the echoed data while ignoring dynamic fields:

POST https://httpbin.org/post
- Content-Type: application/json

```json
{
  "name": "Marcus",
  "type": "api-tester"
}
```

Asserts:
- Status is 200
- Body partially matches:
```json
{
>>  "url": "https://httpbin.org/post",
    "origin": "dynamic-ip-here"
}
```

## Test 3: Multiple fields to check

You can mark multiple lines with `>>` to check several fields:

GET https://httpbin.org/headers
- X-Custom-Header: test-value

Asserts:
- Status is 200
- Body contains `headers`
