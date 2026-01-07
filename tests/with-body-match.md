# Body Match File Tests

These tests verify that Marcus can assert response bodies match external files.

## Test 1: JSON response matches external file

Fetch a JSON response and verify it matches an expected file:

GET https://httpbin.org/json

Assert:
- Status is 200
- Body matches file `expected/slideshow.json`
