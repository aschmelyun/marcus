# API Health Check Tests

These tests verify that our external dependencies are responding correctly.

## Test 1: HTTPBin GET endpoint

The basic GET endpoint should return a 200:

GET https://httpbin.org/get

Assert:
- Status is 200
- Body contains `url`

## Test 2: HTTPBin Headers endpoint

The headers endpoint should also be healthy:

GET https://httpbin.org/headers

Assert:
- Status is 200
- Body contains `headers`

## Test 3: HTTPBin User-Agent endpoint

Check that user-agent endpoint works:

GET https://httpbin.org/user-agent

Assert:
- Status is 200
- Body contains `user-agent`
