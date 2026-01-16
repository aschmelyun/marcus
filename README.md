# Marcus

Human-readable web and API testing from the power and simplicity of Markdown. Easy to use locally or as part of your CI/CD pipeline.

🚧 This is an active-development WIP, breaking changes may occur 🚧

## Installation

```bash
go build -o marcus .
```

## Usage

```bash
# Run tests from a single file
./marcus tests/api.md

# Run all tests in a directory (recursive)
./marcus tests/

# Run tests in parallel
./marcus --parallel tests/

# Quiet mode - only show failures
./marcus --quiet tests/
./marcus -q tests/
```

## Test File Format

Test files are standard markdown. Each test is defined under an H2 (`##`) header.

### Basic Example

```markdown
## Get user list

GET https://api.example.com/users

Assert:
- Status is 200
- Body contains `users`
```

### POST with JSON Body

````markdown
## Create a user

POST https://api.example.com/users
- Content-Type: application/json
- Authorization: Bearer token123

```json
{
  "name": "Alice",
  "email": "alice@example.com"
}
```

Assert:
- Status is 201
- Field `id` equals `1`
- Field `name` equals `"Alice"`
````

### POST with Form Data

````markdown
## Login with credentials

POST https://api.example.com/login
- Content-Type: application/x-www-form-urlencoded

```form
username=alice
password=secret123
remember_me=true
```

Assert:
- Status is 200
- Body contains `token`
````

## Assertions

Assertions are listed under `Assert:` or `Asserts:` as bullet points.

| Assertion | Description |
|-----------|-------------|
| `Status is <code>` | Check HTTP status code |
| `Body contains \`field\`` | Check that a top-level field exists in JSON response |
| `Field \`path\` equals \`value\`` | Check field value using dot notation for nested fields |
| `Body matches file \`path\`` | Compare entire response body against an external file |
| `Duration less than <time>` | Check response time (e.g., `500ms`, `2s`) |

### Field Path Examples

```markdown
Assert:
- Field `name` equals `"Alice"`
- Field `user.email` equals `"alice@example.com"`
- Field `data.items.0.id` equals `123`
- Field `count` equals `42`
- Field `active` equals `true`
```

Values are type-aware: use quotes for strings (`"value"`), no quotes for numbers (`42`) and booleans (`true`/`false`).

### Response Body Matching

Compare the entire response against an external file:

```markdown
## Verify full response structure

GET https://api.example.com/config

Assert:
- Status is 200
- Body matches file `expected/config.json`
```

JSON responses are normalized before comparison, so formatting differences are ignored.

## External File Payloads

For large request bodies, reference an external file instead of inline content:

````markdown
## Create order with large payload

POST https://api.example.com/orders
- Content-Type: application/json

```json
FILE: payloads/order.json
```

Assert:
- Status is 201
````

File paths are relative to the test file's directory.

## Frontmatter

Use YAML frontmatter to set defaults for all tests in a file.

### Base URL

Avoid repeating the full URL in every test:

```markdown
---
root: https://api.example.com/v1
---

## List users

GET /users

Assert:
- Status is 200

## Get specific user

GET /users/123

Assert:
- Status is 200
```

Absolute URLs in individual tests override the root.

### Default Headers

Set headers that apply to all tests in the file:

```markdown
---
headers:
  Accept: application/json
  Authorization: Bearer token123
---

## Get protected resource

GET https://api.example.com/protected

Assert:
- Status is 200
```

Individual tests can override default headers by specifying them explicitly.

### Combined Example

````markdown
---
root: https://api.example.com/v1
headers:
  Accept: application/json
  Authorization: Bearer token123
---

## List all items

GET /items

Assert:
- Status is 200
- Body contains `items`

## Create item

POST /items
- Content-Type: application/json

```json
{
  "name": "New Item"
}
```

Assert:
- Status is 201
````

## Retry and Polling

For async endpoints, wait for a specific status code or field value:

```markdown
## Wait for job completion

GET https://api.example.com/jobs/123
- Wait until status is 200
- Wait until field `state` equals `completed`
- Retry 10 times every 500ms

Assert:
- Status is 200
- Field `state` equals `"completed"`
```

| Option | Default | Description |
|--------|---------|-------------|
| `Wait until status is <code>` | - | Status code to poll for |
| `Wait until field \`path\` equals \`value\`` | - | Field value to poll for |
| `Retry <n> times every <duration>` | 10 times every 1s | Retry configuration |

You can use status and field conditions together—both must be satisfied. The test fails if the conditions aren't met within the retry limit.

## Saving and Reusing Values

Save field values from a response and use them in subsequent tests within the same file:

````markdown
## Create a user

POST https://api.example.com/users
- Content-Type: application/json

```json
{"name": "Alice"}
```

Assert:
- Status is 201

Save:
- Field `id` as `user_id`
- Field `token` as `auth_token`

## Get the created user

GET https://api.example.com/users/{{user_id}}
- Authorization: Bearer {{auth_token}}

Assert:
- Status is 200
- Field `name` equals `"Alice"`
````

### Usage

**Save values** using a `Save:` section after assertions:
```markdown
Save:
- Field `data.id` as `resource_id`
- Field `nested.config.key` as `api_key`
```

**Use values** with `{{variable}}` syntax in URLs, headers, or request bodies:
```markdown
GET /resources/{{resource_id}}
- X-Api-Key: {{api_key}}

```json
{"parent_id": "{{resource_id}}"}
```
```

### Notes

- Variables persist across all tests within a single markdown file
- Variables reset between different test files
- In parallel mode (`--parallel`), variables aren't shared between tests

## Output

```
tests/api.md (3 tests)

  ✓ List users
  ✓ Create user
  ✗ Delete user
    → status assertion failed: expected 204, got 403
       Response: {"error": "Forbidden", "message": "Insufficient permissions"}

2 passed, 1 failed in 1.24s
```

When a status assertion fails, the response body is displayed (up to 500 characters) to help debug the issue. This is especially useful for 4xx/5xx errors that include error messages.

When running a directory:

```
tests/ (2 files, 5 tests)

tests/users.md
  ✓ List users
  ✓ Create user
  312ms

tests/orders.md
  ✓ List orders
  ✓ Create order
  ✓ Cancel order
  523ms

5 passed in 835ms
```

### Quiet Mode

Use `--quiet` or `-q` to suppress output for passing tests. Only failures are shown, and response bodies are hidden from error messages:

```
$ ./marcus --quiet tests/

tests/api.md
  ✗ Delete user
    → status assertion failed: expected 204, got 403
  287ms

4 passed, 1 failed in 835ms
```

When all tests pass, only the summary is displayed:

```
$ ./marcus --quiet tests/

5 passed in 423ms
```

## Project Structure Example

```
project/
├── marcus              # built binary
└── tests/
    ├── users.md
    ├── orders.md
    ├── payloads/
    │   ├── create-user.json
    │   └── create-order.json
    └── expected/
        └── config.json
```

## HTTP Methods

All standard HTTP methods are supported:

- `GET`
- `POST`
- `PUT`
- `PATCH`
- `DELETE`

## Exit Codes

- `0` - All tests passed
- `1` - One or more tests failed

## License

MIT
