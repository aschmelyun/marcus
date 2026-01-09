---
root: https://httpbin.org
---

# Tests with Saved Fields

These tests demonstrate saving field values from one test and using them in subsequent tests.

## Test 1: Make a request and save a field

First, we'll make a request and save a value from the response for use later.

POST /post
- Content-Type: application/json

```json
{"username": "testuser", "action": "create"}
```

Asserts:
- Status is 200
- Body contains `json`

Save:
- Field `json.username` as `saved_username`
- Field `url` as `saved_url`

## Test 2: Use the saved field in a new request

Now we'll use the saved username in the request body.

POST /post
- Content-Type: application/json

```json
{"message": "Hello, {{saved_username}}!", "previous_url": "{{saved_url}}"}
```

Asserts:
- Status is 200
- Field `json.message` equals `Hello, testuser!`
- Field `json.previous_url` equals `https://httpbin.org/post`

## Test 3: Use saved values in URL path

Saved values can also be used in URLs.

GET /anything/users/{{saved_username}}/profile

Asserts:
- Status is 200
- Field `url` equals `https://httpbin.org/anything/users/testuser/profile`

## Test 4: Use saved values in headers

Saved values work in headers too.

GET /headers
- X-Custom-User: {{saved_username}}

Asserts:
- Status is 200
- Field `headers.X-Custom-User` equals `testuser`

## Test 5: Chain multiple saves

Save new values that build on previous ones.

POST /post
- Content-Type: application/json

```json
{"user": "{{saved_username}}", "id": 12345}
```

Asserts:
- Status is 200

Save:
- Field `json.id` as `user_id`

## Test 6: Use all saved values together

Use both the original and newly saved values.

POST /post
- Content-Type: application/json

```json
{"username": "{{saved_username}}", "id": "{{user_id}}", "action": "verify"}
```

Asserts:
- Status is 200
- Field `json.username` equals `testuser`
- Field `json.id` equals `12345`
- Field `json.action` equals `verify`
