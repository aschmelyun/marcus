# API Tests with Expected Failure

This test suite is expected to fail on the second test.

## Test 1: Basic GET endpoint

This should pass:

GET https://httpbin.org/get

Assert:
- Status is 200

## Test 2: Non-existent endpoint

This endpoint returns a 404 and should fail:

GET https://httpbin.org/status/404

Assert:
- Status is 200

## Test 3: This test will never run

Because test 2 fails, this one is skipped:

GET https://httpbin.org/headers

Assert:
- Status is 200
