package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// captureOutput captures stdout during the execution of a function
func captureOutput(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	f()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name            string
		content         string
		expectedRoot    string
		expectedHeaders map[string]string
		expectContent   string
	}{
		{
			name: "basic frontmatter with headers",
			content: `---
headers:
  Accept: application/json
  X-Custom: test
---

## Test 1`,
			expectedHeaders: map[string]string{
				"Accept":   "application/json",
				"X-Custom": "test",
			},
			expectContent: "\n## Test 1",
		},
		{
			name:            "no frontmatter",
			content:         "## Test 1\nGET https://example.com",
			expectedHeaders: map[string]string{},
			expectContent:   "## Test 1\nGET https://example.com",
		},
		{
			name: "frontmatter without headers section",
			content: `---
something: else
---

## Test 1`,
			expectedHeaders: map[string]string{},
			expectContent:   "\n## Test 1",
		},
		{
			name: "unclosed frontmatter",
			content: `---
headers:
  Accept: application/json
## Test 1`,
			expectedHeaders: map[string]string{},
			expectContent: `---
headers:
  Accept: application/json
## Test 1`,
		},
		{
			name: "frontmatter with root",
			content: `---
root: https://api.example.com
---

## Test 1`,
			expectedRoot:    "https://api.example.com",
			expectedHeaders: map[string]string{},
			expectContent:   "\n## Test 1",
		},
		{
			name: "frontmatter with root and headers",
			content: `---
root: https://api.example.com/v2
headers:
  Authorization: Bearer token
---

## Test 1`,
			expectedRoot: "https://api.example.com/v2",
			expectedHeaders: map[string]string{
				"Authorization": "Bearer token",
			},
			expectContent: "\n## Test 1",
		},
		{
			name: "root with trailing slash is trimmed",
			content: `---
root: https://api.example.com/
---

## Test 1`,
			expectedRoot:    "https://api.example.com",
			expectedHeaders: map[string]string{},
			expectContent:   "\n## Test 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defaults, remaining := parseFrontmatter(tt.content)

			if defaults.Root != tt.expectedRoot {
				t.Errorf("root: expected %q, got %q", tt.expectedRoot, defaults.Root)
			}

			if len(defaults.Headers) != len(tt.expectedHeaders) {
				t.Errorf("expected %d headers, got %d", len(tt.expectedHeaders), len(defaults.Headers))
			}

			for key, expectedVal := range tt.expectedHeaders {
				if gotVal, ok := defaults.Headers[key]; !ok {
					t.Errorf("missing header %q", key)
				} else if gotVal != expectedVal {
					t.Errorf("header %q: expected %q, got %q", key, expectedVal, gotVal)
				}
			}

			if remaining != tt.expectContent {
				t.Errorf("remaining content mismatch\nexpected: %q\ngot: %q", tt.expectContent, remaining)
			}
		})
	}
}

func TestParseTests(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		expectedCount int
		expectedNames []string
	}{
		{
			name: "single test",
			content: `## Test 1
GET https://httpbin.org/get

Asserts:
- Status is 200`,
			expectedCount: 1,
			expectedNames: []string{"Test 1"},
		},
		{
			name: "multiple tests",
			content: `## First Test
GET https://httpbin.org/get

## Second Test
POST https://httpbin.org/post

## Third Test
DELETE https://httpbin.org/delete`,
			expectedCount: 3,
			expectedNames: []string{"First Test", "Second Test", "Third Test"},
		},
		{
			name:          "no tests",
			content:       "# Just a heading\nSome content",
			expectedCount: 0,
			expectedNames: []string{},
		},
		{
			name: "test with frontmatter",
			content: `---
headers:
  Accept: application/json
---

## Test With Defaults
GET https://httpbin.org/get`,
			expectedCount: 1,
			expectedNames: []string{"Test With Defaults"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTests(tt.content, "")

			if len(result) != tt.expectedCount {
				t.Errorf("expected %d tests, got %d", tt.expectedCount, len(result))
			}

			for i, expectedName := range tt.expectedNames {
				if i < len(result) && result[i].Name != expectedName {
					t.Errorf("test %d: expected name %q, got %q", i, expectedName, result[i].Name)
				}
			}
		})
	}
}

func TestParseTestBlock(t *testing.T) {
	defaults := Defaults{Headers: make(map[string]string)}

	tests := []struct {
		name           string
		blockName      string
		content        string
		defaults       Defaults
		baseDir        string
		expectedMethod string
		expectedURL    string
		expectedHeader map[string]string
		expectedBody   string
	}{
		{
			name:           "GET request",
			blockName:      "Simple GET",
			content:        "GET https://httpbin.org/get",
			defaults:       defaults,
			baseDir:        "",
			expectedMethod: "GET",
			expectedURL:    "https://httpbin.org/get",
			expectedHeader: map[string]string{},
			expectedBody:   "",
		},
		{
			name:      "POST with headers",
			blockName: "POST Test",
			content: `POST https://httpbin.org/post
- Content-Type: application/json
- Authorization: Bearer token`,
			defaults:       defaults,
			baseDir:        "",
			expectedMethod: "POST",
			expectedURL:    "https://httpbin.org/post",
			expectedHeader: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": "Bearer token",
			},
			expectedBody: "",
		},
		{
			name:      "with default headers",
			blockName: "With Defaults",
			content:   "GET https://httpbin.org/get",
			defaults: Defaults{
				Headers: map[string]string{
					"Accept": "application/json",
				},
			},
			baseDir:        "",
			expectedMethod: "GET",
			expectedURL:    "https://httpbin.org/get",
			expectedHeader: map[string]string{
				"Accept": "application/json",
			},
			expectedBody: "",
		},
		{
			name:      "override default header",
			blockName: "Override Default",
			content: `GET https://httpbin.org/get
- Accept: text/plain`,
			defaults: Defaults{
				Headers: map[string]string{
					"Accept": "application/json",
				},
			},
			baseDir:        "",
			expectedMethod: "GET",
			expectedURL:    "https://httpbin.org/get",
			expectedHeader: map[string]string{
				"Accept": "text/plain",
			},
			expectedBody: "",
		},
		{
			name:      "relative path with root",
			blockName: "Relative Path",
			content:   "GET /users",
			defaults: Defaults{
				Root:    "https://api.example.com",
				Headers: map[string]string{},
			},
			baseDir:        "",
			expectedMethod: "GET",
			expectedURL:    "https://api.example.com/users",
			expectedHeader: map[string]string{},
			expectedBody:   "",
		},
		{
			name:      "relative path with root containing path",
			blockName: "Relative with API path",
			content:   "POST /users",
			defaults: Defaults{
				Root:    "https://api.example.com/v2",
				Headers: map[string]string{},
			},
			baseDir:        "",
			expectedMethod: "POST",
			expectedURL:    "https://api.example.com/v2/users",
			expectedHeader: map[string]string{},
			expectedBody:   "",
		},
		{
			name:      "absolute URL ignores root",
			blockName: "Absolute URL",
			content:   "GET https://other.com/path",
			defaults: Defaults{
				Root:    "https://api.example.com",
				Headers: map[string]string{},
			},
			baseDir:        "",
			expectedMethod: "GET",
			expectedURL:    "https://other.com/path",
			expectedHeader: map[string]string{},
			expectedBody:   "",
		},
		{
			name:      "FILE: payload reference",
			blockName: "File Payload",
			content: "POST https://httpbin.org/post\n\n```json\nFILE: payload.json\n```",
			defaults:       defaults,
			baseDir:        "testdata",
			expectedMethod: "POST",
			expectedURL:    "https://httpbin.org/post",
			expectedHeader: map[string]string{},
			expectedBody:   "{\"test\": \"data\"}\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTestBlock(tt.blockName, tt.content, tt.defaults, tt.baseDir)

			if result.Method != tt.expectedMethod {
				t.Errorf("expected method %q, got %q", tt.expectedMethod, result.Method)
			}

			if result.URL != tt.expectedURL {
				t.Errorf("expected URL %q, got %q", tt.expectedURL, result.URL)
			}

			if len(result.Headers) != len(tt.expectedHeader) {
				t.Errorf("expected %d headers, got %d", len(tt.expectedHeader), len(result.Headers))
			}

			for key, expectedVal := range tt.expectedHeader {
				if gotVal, ok := result.Headers[key]; !ok {
					t.Errorf("missing header %q", key)
				} else if gotVal != expectedVal {
					t.Errorf("header %q: expected %q, got %q", key, expectedVal, gotVal)
				}
			}

			if result.Body != tt.expectedBody {
				t.Errorf("expected body %q, got %q", tt.expectedBody, result.Body)
			}
		})
	}
}

func TestParseTestBlockRetryOptions(t *testing.T) {
	defaults := Defaults{Headers: make(map[string]string)}

	tests := []struct {
		name                string
		content             string
		expectedWaitFor     int
		expectedWaitField   string
		expectedWaitValue   string
		expectedRetryDelay  time.Duration
		expectedRetryMax    int
	}{
		{
			name: "wait and retry options",
			content: `GET https://example.com/status
- Wait until status is 200
- Retry 5 times every 500ms`,
			expectedWaitFor:    200,
			expectedRetryDelay: 500 * time.Millisecond,
			expectedRetryMax:   5,
		},
		{
			name: "only wait for status",
			content: `GET https://example.com/status
- Wait until status is 201`,
			expectedWaitFor:    201,
			expectedRetryDelay: 0,
			expectedRetryMax:   0,
		},
		{
			name: "retry with seconds delay",
			content: `GET https://example.com/status
- Wait until status is 200
- Retry 3 times every 2s`,
			expectedWaitFor:    200,
			expectedRetryDelay: 2 * time.Second,
			expectedRetryMax:   3,
		},
		{
			name: "mixed headers and retry options",
			content: `GET https://example.com/status
- Authorization: Bearer token
- Wait until status is 200
- Content-Type: application/json
- Retry 3 times every 1s`,
			expectedWaitFor:    200,
			expectedRetryDelay: 1 * time.Second,
			expectedRetryMax:   3,
		},
		{
			name:               "no retry options",
			content:            "GET https://example.com/get",
			expectedWaitFor:    0,
			expectedRetryDelay: 0,
			expectedRetryMax:   0,
		},
		{
			name: "wait for field equals",
			content: "GET https://example.com/status\n- Wait until field `status.code` equals `ready`",
			expectedWaitField: "status.code",
			expectedWaitValue: "ready",
		},
		{
			name: "wait for field with retry options",
			content: "GET https://example.com/status\n- Wait until field `message.state` equals `completed`\n- Retry 5 times every 2s",
			expectedWaitField:  "message.state",
			expectedWaitValue:  "completed",
			expectedRetryDelay: 2 * time.Second,
			expectedRetryMax:   5,
		},
		{
			name: "wait for both status and field",
			content: `GET https://example.com/status
- Wait until status is 200
- Wait until field ` + "`result`" + ` equals ` + "`success`",
			expectedWaitFor:   200,
			expectedWaitField: "result",
			expectedWaitValue: "success",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTestBlock("Test", tt.content, defaults, "")

			if result.WaitForStatus != tt.expectedWaitFor {
				t.Errorf("WaitForStatus: expected %d, got %d", tt.expectedWaitFor, result.WaitForStatus)
			}

			if result.WaitForField != tt.expectedWaitField {
				t.Errorf("WaitForField: expected %q, got %q", tt.expectedWaitField, result.WaitForField)
			}

			if result.WaitForValue != tt.expectedWaitValue {
				t.Errorf("WaitForValue: expected %q, got %q", tt.expectedWaitValue, result.WaitForValue)
			}

			if result.RetryDelay != tt.expectedRetryDelay {
				t.Errorf("RetryDelay: expected %v, got %v", tt.expectedRetryDelay, result.RetryDelay)
			}

			if result.RetryMax != tt.expectedRetryMax {
				t.Errorf("RetryMax: expected %d, got %d", tt.expectedRetryMax, result.RetryMax)
			}
		})
	}
}

func TestParseAssertions(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []Assertion
	}{
		{
			name: "status assertion",
			content: `Asserts:
- Status is 200`,
			expected: []Assertion{
				{Type: "status", Value: "200"},
			},
		},
		{
			name: "body contains assertion",
			content: `Asserts:
- Body contains ` + "`url`",
			expected: []Assertion{
				{Type: "body_contains", Field: "url"},
			},
		},
		{
			name: "field equals assertion",
			content: `Asserts:
- Field ` + "`json.name`" + ` equals ` + "`test`",
			expected: []Assertion{
				{Type: "field_equals", Field: "json.name", Value: "test"},
			},
		},
		{
			name: "multiple assertions",
			content: `Asserts:
- Status is 201
- Body contains ` + "`id`" + `
- Field ` + "`data.type`" + ` equals ` + "`user`",
			expected: []Assertion{
				{Type: "status", Value: "201"},
				{Type: "body_contains", Field: "id"},
				{Type: "field_equals", Field: "data.type", Value: "user"},
			},
		},
		{
			name: "duration assertion with ms",
			content: `Asserts:
- Duration less than 500ms`,
			expected: []Assertion{
				{Type: "duration", Value: "500ms"},
			},
		},
		{
			name: "duration assertion with seconds",
			content: `Asserts:
- Time less than 2s`,
			expected: []Assertion{
				{Type: "duration", Value: "2s"},
			},
		},
		{
			name:     "no assertions section",
			content:  "GET https://example.com",
			expected: []Assertion{},
		},
		{
			name: "body matches file assertion",
			content: `Asserts:
- Body matches file ` + "`expected/response.json`",
			expected: []Assertion{
				{Type: "body_matches_file", Value: "expected/response.json"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAssertions(tt.content, "")

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d assertions, got %d", len(tt.expected), len(result))
				return
			}

			for i, exp := range tt.expected {
				if result[i].Type != exp.Type {
					t.Errorf("assertion %d: expected type %q, got %q", i, exp.Type, result[i].Type)
				}
				if result[i].Field != exp.Field {
					t.Errorf("assertion %d: expected field %q, got %q", i, exp.Field, result[i].Field)
				}
				if result[i].Value != exp.Value {
					t.Errorf("assertion %d: expected value %q, got %q", i, exp.Value, result[i].Value)
				}
			}
		})
	}
}

func TestGetJSONField(t *testing.T) {
	data := map[string]interface{}{
		"name": "test",
		"nested": map[string]interface{}{
			"value": "deep",
			"level2": map[string]interface{}{
				"item": "found",
			},
		},
		"number": float64(42),
	}

	tests := []struct {
		name        string
		path        string
		expected    interface{}
		expectError bool
	}{
		{
			name:     "top level field",
			path:     "name",
			expected: "test",
		},
		{
			name:     "nested field",
			path:     "nested.value",
			expected: "deep",
		},
		{
			name:     "deeply nested field",
			path:     "nested.level2.item",
			expected: "found",
		},
		{
			name:     "number field",
			path:     "number",
			expected: float64(42),
		},
		{
			name:        "non-existent field",
			path:        "missing",
			expectError: true,
		},
		{
			name:        "non-existent nested field",
			path:        "nested.missing",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := getJSONField(data, tt.path)

			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestParseExpectedValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected interface{}
	}{
		{name: "integer", input: "42", expected: int64(42)},
		{name: "negative integer", input: "-10", expected: int64(-10)},
		{name: "float", input: "3.14", expected: 3.14},
		{name: "boolean true", input: "true", expected: true},
		{name: "boolean false", input: "false", expected: false},
		{name: "quoted string", input: `"hello"`, expected: "hello"},
		{name: "plain string", input: "hello", expected: "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseExpectedValue(tt.input)
			if result != tt.expected {
				t.Errorf("expected %v (%T), got %v (%T)", tt.expected, tt.expected, result, result)
			}
		})
	}
}

func TestValuesEqual(t *testing.T) {
	tests := []struct {
		name     string
		actual   interface{}
		expected interface{}
		equal    bool
	}{
		{name: "equal strings", actual: "test", expected: "test", equal: true},
		{name: "different strings", actual: "test", expected: "other", equal: false},
		{name: "equal numbers", actual: float64(42), expected: int64(42), equal: true},
		{name: "equal booleans", actual: true, expected: true, equal: true},
		{name: "string and number", actual: "42", expected: int64(42), equal: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := valuesEqual(tt.actual, tt.expected)
			if result != tt.equal {
				t.Errorf("expected %v, got %v", tt.equal, result)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		ms       int64
		expected string
	}{
		{name: "milliseconds", ms: 500, expected: "500ms"},
		{name: "one second", ms: 1000, expected: "1.00s"},
		{name: "seconds with decimals", ms: 2500, expected: "2.50s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(time.Duration(tt.ms) * time.Millisecond)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectedMs  int64
		expectError bool
	}{
		{name: "milliseconds", input: "500ms", expectedMs: 500},
		{name: "seconds", input: "2s", expectedMs: 2000},
		{name: "seconds with decimal", input: "1.5s", expectedMs: 1500},
		{name: "with whitespace", input: " 100ms ", expectedMs: 100},
		{name: "invalid", input: "abc", expectError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseDuration(tt.input)

			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			expectedDuration := time.Duration(tt.expectedMs) * time.Millisecond
			if result != expectedDuration {
				t.Errorf("expected %v, got %v", expectedDuration, result)
			}
		})
	}
}

func TestParseSaveFields(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []SaveField
	}{
		{
			name: "single save field",
			content: "Save:\n- Field `json.id` as `user_id`",
			expected: []SaveField{
				{Field: "json.id", Variable: "user_id"},
			},
		},
		{
			name: "multiple save fields",
			content: "Save:\n- Field `data.id` as `id`\n- Field `data.token` as `auth_token`",
			expected: []SaveField{
				{Field: "data.id", Variable: "id"},
				{Field: "data.token", Variable: "auth_token"},
			},
		},
		{
			name: "saves plural section",
			content: "Saves:\n- Field `response.key` as `api_key`",
			expected: []SaveField{
				{Field: "response.key", Variable: "api_key"},
			},
		},
		{
			name:     "no save section",
			content:  "Asserts:\n- Status is 200",
			expected: []SaveField{},
		},
		{
			name: "nested field path",
			content: "Save:\n- Field `data.user.profile.id` as `profile_id`",
			expected: []SaveField{
				{Field: "data.user.profile.id", Variable: "profile_id"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseSaveFields(tt.content)

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d save fields, got %d", len(tt.expected), len(result))
				return
			}

			for i, exp := range tt.expected {
				if result[i].Field != exp.Field {
					t.Errorf("save field %d: expected field %q, got %q", i, exp.Field, result[i].Field)
				}
				if result[i].Variable != exp.Variable {
					t.Errorf("save field %d: expected variable %q, got %q", i, exp.Variable, result[i].Variable)
				}
			}
		})
	}
}

func TestInterpolateVariables(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		vars     map[string]interface{}
		expected string
	}{
		{
			name:     "single variable",
			input:    "https://api.example.com/users/{{user_id}}",
			vars:     map[string]interface{}{"user_id": "123"},
			expected: "https://api.example.com/users/123",
		},
		{
			name:     "multiple variables",
			input:    "{{base_url}}/users/{{user_id}}/posts/{{post_id}}",
			vars:     map[string]interface{}{"base_url": "https://api.example.com", "user_id": "42", "post_id": "99"},
			expected: "https://api.example.com/users/42/posts/99",
		},
		{
			name:     "no variables",
			input:    "https://api.example.com/users",
			vars:     map[string]interface{}{"unused": "value"},
			expected: "https://api.example.com/users",
		},
		{
			name:     "nil vars map",
			input:    "https://api.example.com/{{id}}",
			vars:     nil,
			expected: "https://api.example.com/{{id}}",
		},
		{
			name:     "numeric variable",
			input:    "ID is {{id}}",
			vars:     map[string]interface{}{"id": 42},
			expected: "ID is 42",
		},
		{
			name:     "variable in header value",
			input:    "Bearer {{token}}",
			vars:     map[string]interface{}{"token": "abc123"},
			expected: "Bearer abc123",
		},
		{
			name:     "variable in JSON body",
			input:    `{"parent_id": "{{parent_id}}", "name": "test"}`,
			vars:     map[string]interface{}{"parent_id": "xyz"},
			expected: `{"parent_id": "xyz", "name": "test"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := interpolateVariables(tt.input, tt.vars)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestRunTestsSequentialQuietMode(t *testing.T) {
	// Create test files with passing tests
	passingTests := []TestFile{
		{
			Path: "test.md",
			Tests: []Test{
				{
					Name:   "Passing Test 1",
					Method: "GET",
					URL:    "https://httpbin.org/status/200",
					Assertions: []Assertion{
						{Type: "status", Value: "200"},
					},
				},
				{
					Name:   "Passing Test 2",
					Method: "GET",
					URL:    "https://httpbin.org/status/200",
					Assertions: []Assertion{
						{Type: "status", Value: "200"},
					},
				},
			},
		},
	}

	t.Run("quiet mode hides passing tests", func(t *testing.T) {
		output := captureOutput(func() {
			runTestsSequential(passingTests, true)
		})

		// In quiet mode with all passing, output should NOT contain test names
		if strings.Contains(output, "Passing Test 1") {
			t.Error("quiet mode should not show passing test names")
		}
		if strings.Contains(output, "Passing Test 2") {
			t.Error("quiet mode should not show passing test names")
		}
		// Should not contain checkmarks
		if strings.Contains(output, "✓") {
			t.Error("quiet mode should not show checkmarks for passing tests")
		}
	})

	t.Run("normal mode shows passing tests", func(t *testing.T) {
		output := captureOutput(func() {
			runTestsSequential(passingTests, false)
		})

		// In normal mode, output should contain test names
		if !strings.Contains(output, "Passing Test 1") {
			t.Error("normal mode should show passing test names")
		}
		if !strings.Contains(output, "Passing Test 2") {
			t.Error("normal mode should show passing test names")
		}
		// Should contain checkmarks
		if !strings.Contains(output, "✓") {
			t.Error("normal mode should show checkmarks for passing tests")
		}
	})
}

func TestRunTestsSequentialQuietModeWithFailures(t *testing.T) {
	// Create test files with a mix of passing and failing tests
	mixedTests := []TestFile{
		{
			Path: "test.md",
			Tests: []Test{
				{
					Name:   "Passing Test",
					Method: "GET",
					URL:    "https://httpbin.org/status/200",
					Assertions: []Assertion{
						{Type: "status", Value: "200"},
					},
				},
				{
					Name:   "Failing Test",
					Method: "GET",
					URL:    "https://httpbin.org/status/404",
					Assertions: []Assertion{
						{Type: "status", Value: "200"},
					},
				},
			},
		},
	}

	t.Run("quiet mode shows failing tests only", func(t *testing.T) {
		output := captureOutput(func() {
			runTestsSequential(mixedTests, true)
		})

		// Should NOT show passing test
		if strings.Contains(output, "Passing Test") {
			t.Error("quiet mode should not show passing test names")
		}
		// Should show failing test
		if !strings.Contains(output, "Failing Test") {
			t.Error("quiet mode should show failing test names")
		}
		// Should contain X mark for failure
		if !strings.Contains(output, "✗") {
			t.Error("quiet mode should show X marks for failing tests")
		}
		// Should contain error message
		if !strings.Contains(output, "status assertion failed") {
			t.Error("quiet mode should show error messages")
		}
	})
}

func TestRunTestsParallelQuietMode(t *testing.T) {
	// Create test files with passing tests
	passingTests := []TestFile{
		{
			Path: "test.md",
			Tests: []Test{
				{
					Name:   "Parallel Passing 1",
					Method: "GET",
					URL:    "https://httpbin.org/status/200",
					Assertions: []Assertion{
						{Type: "status", Value: "200"},
					},
				},
				{
					Name:   "Parallel Passing 2",
					Method: "GET",
					URL:    "https://httpbin.org/status/200",
					Assertions: []Assertion{
						{Type: "status", Value: "200"},
					},
				},
			},
		},
	}

	t.Run("quiet mode hides passing tests in parallel", func(t *testing.T) {
		output := captureOutput(func() {
			runTestsParallel(passingTests, true)
		})

		// In quiet mode with all passing, output should NOT contain test names
		if strings.Contains(output, "Parallel Passing 1") {
			t.Error("quiet mode should not show passing test names")
		}
		if strings.Contains(output, "Parallel Passing 2") {
			t.Error("quiet mode should not show passing test names")
		}
	})

	t.Run("normal mode shows passing tests in parallel", func(t *testing.T) {
		output := captureOutput(func() {
			runTestsParallel(passingTests, false)
		})

		// In normal mode, output should contain test names
		if !strings.Contains(output, "Parallel Passing 1") {
			t.Error("normal mode should show passing test names")
		}
		if !strings.Contains(output, "Parallel Passing 2") {
			t.Error("normal mode should show passing test names")
		}
	})
}

func TestOnlyFlagFiltersTests(t *testing.T) {
	// Create test files with multiple tests
	testFiles := []TestFile{
		{
			Path: "test.md",
			Tests: []Test{
				{
					Name:   "Test 1",
					Method: "GET",
					URL:    "https://httpbin.org/status/200",
					Assertions: []Assertion{
						{Type: "status", Value: "200"},
					},
				},
				{
					Name:   "Test 2",
					Method: "GET",
					URL:    "https://httpbin.org/status/200",
					Assertions: []Assertion{
						{Type: "status", Value: "200"},
					},
				},
				{
					Name:   "Test 3",
					Method: "GET",
					URL:    "https://httpbin.org/status/200",
					Assertions: []Assertion{
						{Type: "status", Value: "200"},
					},
				},
			},
		},
	}

	t.Run("filter to second test only", func(t *testing.T) {
		// Simulate --only=2 filtering
		only := 2
		totalTests := 0
		for _, tf := range testFiles {
			totalTests += len(tf.Tests)
		}

		if only > totalTests {
			t.Fatal("test index out of range")
		}

		// Find the test at position 'only' (1-indexed)
		var filteredFiles []TestFile
		testNum := 0
		for _, tf := range testFiles {
			for _, test := range tf.Tests {
				testNum++
				if testNum == only {
					filteredFiles = []TestFile{{Path: tf.Path, Tests: []Test{test}}}
					break
				}
			}
		}

		if len(filteredFiles) != 1 {
			t.Errorf("expected 1 test file, got %d", len(filteredFiles))
		}
		if len(filteredFiles[0].Tests) != 1 {
			t.Errorf("expected 1 test, got %d", len(filteredFiles[0].Tests))
		}
		if filteredFiles[0].Tests[0].Name != "Test 2" {
			t.Errorf("expected 'Test 2', got %q", filteredFiles[0].Tests[0].Name)
		}
	})

	t.Run("filter to first test", func(t *testing.T) {
		only := 1
		testNum := 0
		var filteredFiles []TestFile
		for _, tf := range testFiles {
			for _, test := range tf.Tests {
				testNum++
				if testNum == only {
					filteredFiles = []TestFile{{Path: tf.Path, Tests: []Test{test}}}
					break
				}
			}
		}

		if filteredFiles[0].Tests[0].Name != "Test 1" {
			t.Errorf("expected 'Test 1', got %q", filteredFiles[0].Tests[0].Name)
		}
	})

	t.Run("filter to last test", func(t *testing.T) {
		only := 3
		testNum := 0
		var filteredFiles []TestFile
		for _, tf := range testFiles {
			for _, test := range tf.Tests {
				testNum++
				if testNum == only {
					filteredFiles = []TestFile{{Path: tf.Path, Tests: []Test{test}}}
					break
				}
			}
		}

		if filteredFiles[0].Tests[0].Name != "Test 3" {
			t.Errorf("expected 'Test 3', got %q", filteredFiles[0].Tests[0].Name)
		}
	})

	t.Run("out of range returns error condition", func(t *testing.T) {
		only := 5
		totalTests := 0
		for _, tf := range testFiles {
			totalTests += len(tf.Tests)
		}

		if only <= totalTests {
			t.Error("test index should be out of range")
		}
	})
}

func TestStatusFailureShowsResponseBody(t *testing.T) {
	// This test uses an endpoint that returns a JSON body
	// We assert the wrong status to trigger a failure with body content
	failingTests := []TestFile{
		{
			Path: "test.md",
			Tests: []Test{
				{
					Name:   "Status Mismatch Test",
					Method: "POST",
					URL:    "https://httpbin.org/post",
					Assertions: []Assertion{
						{Type: "status", Value: "201"}, // Will get 200, so this fails
					},
				},
			},
		},
	}

	t.Run("normal mode shows response body on status failure", func(t *testing.T) {
		output := captureOutput(func() {
			runTestsSequential(failingTests, false)
		})

		// Should show the status mismatch
		if !strings.Contains(output, "expected 201, got 200") {
			t.Error("should show status mismatch")
		}
		// Should show "Response:" with body content
		if !strings.Contains(output, "Response:") {
			t.Error("normal mode should show Response: label")
		}
		// httpbin.org/post returns JSON with "url" field
		if !strings.Contains(output, "httpbin.org/post") {
			t.Error("normal mode should show response body content")
		}
	})

	t.Run("quiet mode hides response body on status failure", func(t *testing.T) {
		output := captureOutput(func() {
			runTestsSequential(failingTests, true)
		})

		// Should still show the status mismatch
		if !strings.Contains(output, "expected 201, got 200") {
			t.Error("should show status mismatch")
		}
		// Should NOT show "Response:" or body content
		if strings.Contains(output, "Response:") {
			t.Error("quiet mode should not show Response: label")
		}
	})
}
