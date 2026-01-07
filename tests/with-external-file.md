# External File Payload Tests

These tests verify that Marcus can load request bodies from external files.

## Test 1: POST with external JSON payload

Create a user using a JSON payload loaded from an external file:

POST https://httpbin.org/post
- Content-Type: application/json

```json
FILE: payloads/user.json
```

Assert:
- Status is 200
- Body contains `json`
- Field `json.username` equals `"external_file_user"`
- Field `json.email` equals `"external@example.com"`
- Field `json.profile.firstName` equals `"External"`
