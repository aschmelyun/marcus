# User Management API

This test suite validates the core user management endpoints of our API. It covers user creation, retrieval, authentication, and profile updates. These endpoints form the foundation of our identity system and must remain stable across releases.

## Test 1: Fetch User List

Retrieve a paginated list of users from the system. This endpoint is typically used by admin dashboards and user search features.

GET https://httpbin.org/get?page=1&limit=10

Assert:
- Status is 200
- Body contains `args`
- Field `args.page` equals `"1"`
- Field `args.limit` equals `"10"`

## Test 2: Create New User

Register a new user account with the required profile information. The API should validate all fields and return the created user object with a generated ID.

POST https://httpbin.org/post
- Content-Type: application/json

```json
{
  "username": "marcus_test_user",
  "email": "marcus@example.com",
  "password": "securePassword123",
  "profile": {
    "firstName": "Marcus",
    "lastName": "Aurelius",
    "role": "developer"
  }
}
```

Asserts:
- Status is 200
- Body contains `json`
- Field `json.username` equals `"marcus_test_user"`
- Field `json.email` equals `"marcus@example.com"`
- Field `json.profile.firstName` equals `"Marcus"`

## Test 3: User Login with Form Data

Authenticate a user using traditional form-encoded credentials. This endpoint supports legacy clients that don't send JSON payloads.

POST https://httpbin.org/post
- Content-Type: application/x-www-form-urlencoded

```form
username=marcus_test_user
password=securePassword123
remember_me=true
```

Asserts:
- Status is 200
- Body contains `form`
- Field `form.username` equals `"marcus_test_user"`
- Field `form.remember_me` equals `"true"`