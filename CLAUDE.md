# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Marcus is a markdown-based API testing tool written in Go. It parses markdown files containing API test definitions and executes them, validating responses against assertions.

## Build & Run Commands

```bash
# Build the binary
go build -o marcus .

# Run tests against a markdown test file
./marcus tests/passing.md      # Expected to pass
./marcus tests/failing.md      # Expected to fail (for testing failure handling)
./marcus tests/user-api.md     # User API test suite
```

## Markdown Test File Format

Test files use a specific markdown structure:

- `## Test Name` - Each test starts with an H2 header
- `GET/POST/PUT/PATCH/DELETE https://url` - HTTP method and URL on a single line
- `- Header-Name: value` - Headers as bullet points after the URL
- ` ```json ` or ` ```form ` - Code blocks for request body (inline or via `FILE:` reference)
- `Assert:` or `Asserts:` - Assertion section with bullet points

### External File Payloads

For large payloads, use `FILE:` to reference an external file instead of inline content:

```json
FILE: payloads/user.json
```

Paths are relative to the test file's directory.

### Supported Assertions

Assertions should be written in plain, human-readable English. Avoid symbols like `<`, `>`, `=` in assertion syntax—use words instead (e.g., "less than" not "<", "equals" not "=").

- `Status is <code>` - HTTP status code check
- `Body contains \`field\`` - Checks for field presence in JSON response
- `Field \`path.to.field\` equals \`value\`` - Checks field value (supports dot notation for nested fields)
- `Body matches file \`path/to/file.json\`` - Compares entire response body against external file (JSON is normalized before comparison)
- `Duration less than <time>` - Checks response time (e.g., `500ms`, `2s`)

## Architecture

Single-file Go application (`main.go`) with these components:

- **Parsing**: `parseTests()` splits markdown by `##` headers, `parseTestBlock()` extracts method/URL/headers/body, `parseAssertions()` handles the assertion section
- **Execution**: `runTest()` builds and executes HTTP requests, handles JSON and form-encoded bodies
- **Validation**: `validateAssertion()` checks status codes, body fields, and field equality using `getJSONField()` for dot-notation path traversal
