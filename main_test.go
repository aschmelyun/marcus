package main

import (
	"testing"
	"time"
)

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
			result := parseTests(tt.content)

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
		expectedMethod string
		expectedURL    string
		expectedHeader map[string]string
	}{
		{
			name:           "GET request",
			blockName:      "Simple GET",
			content:        "GET https://httpbin.org/get",
			defaults:       defaults,
			expectedMethod: "GET",
			expectedURL:    "https://httpbin.org/get",
			expectedHeader: map[string]string{},
		},
		{
			name:      "POST with headers",
			blockName: "POST Test",
			content: `POST https://httpbin.org/post
- Content-Type: application/json
- Authorization: Bearer token`,
			defaults:       defaults,
			expectedMethod: "POST",
			expectedURL:    "https://httpbin.org/post",
			expectedHeader: map[string]string{
				"Content-Type":  "application/json",
				"Authorization": "Bearer token",
			},
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
			expectedMethod: "GET",
			expectedURL:    "https://httpbin.org/get",
			expectedHeader: map[string]string{
				"Accept": "application/json",
			},
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
			expectedMethod: "GET",
			expectedURL:    "https://httpbin.org/get",
			expectedHeader: map[string]string{
				"Accept": "text/plain",
			},
		},
		{
			name:      "relative path with root",
			blockName: "Relative Path",
			content:   "GET /users",
			defaults: Defaults{
				Root:    "https://api.example.com",
				Headers: map[string]string{},
			},
			expectedMethod: "GET",
			expectedURL:    "https://api.example.com/users",
			expectedHeader: map[string]string{},
		},
		{
			name:      "relative path with root containing path",
			blockName: "Relative with API path",
			content:   "POST /users",
			defaults: Defaults{
				Root:    "https://api.example.com/v2",
				Headers: map[string]string{},
			},
			expectedMethod: "POST",
			expectedURL:    "https://api.example.com/v2/users",
			expectedHeader: map[string]string{},
		},
		{
			name:      "absolute URL ignores root",
			blockName: "Absolute URL",
			content:   "GET https://other.com/path",
			defaults: Defaults{
				Root:    "https://api.example.com",
				Headers: map[string]string{},
			},
			expectedMethod: "GET",
			expectedURL:    "https://other.com/path",
			expectedHeader: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTestBlock(tt.blockName, tt.content, tt.defaults)

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
		})
	}
}

func TestParseTestBlockRetryOptions(t *testing.T) {
	defaults := Defaults{Headers: make(map[string]string)}

	tests := []struct {
		name              string
		content           string
		expectedWaitFor   int
		expectedRetryDelay time.Duration
		expectedRetryMax  int
	}{
		{
			name: "all retry options",
			content: `GET https://example.com/status
- Wait for status: 200
- Retry-Delay: 500ms
- Retry-Max: 5`,
			expectedWaitFor:   200,
			expectedRetryDelay: 500 * time.Millisecond,
			expectedRetryMax:  5,
		},
		{
			name: "only wait for status",
			content: `GET https://example.com/status
- Wait for status: 201`,
			expectedWaitFor:   201,
			expectedRetryDelay: 0,
			expectedRetryMax:  0,
		},
		{
			name: "retry with seconds delay",
			content: `GET https://example.com/status
- Wait for status: 200
- Retry-Delay: 2s`,
			expectedWaitFor:   200,
			expectedRetryDelay: 2 * time.Second,
			expectedRetryMax:  0,
		},
		{
			name: "mixed headers and retry options",
			content: `GET https://example.com/status
- Authorization: Bearer token
- Wait for status: 200
- Content-Type: application/json
- Retry-Max: 3`,
			expectedWaitFor:   200,
			expectedRetryDelay: 0,
			expectedRetryMax:  3,
		},
		{
			name:              "no retry options",
			content:           "GET https://example.com/get",
			expectedWaitFor:   0,
			expectedRetryDelay: 0,
			expectedRetryMax:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTestBlock("Test", tt.content, defaults)

			if result.WaitForStatus != tt.expectedWaitFor {
				t.Errorf("WaitForStatus: expected %d, got %d", tt.expectedWaitFor, result.WaitForStatus)
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
- Duration < 500ms`,
			expected: []Assertion{
				{Type: "duration", Value: "500ms"},
			},
		},
		{
			name: "duration assertion with seconds",
			content: `Asserts:
- Time < 2s`,
			expected: []Assertion{
				{Type: "duration", Value: "2s"},
			},
		},
		{
			name:     "no assertions section",
			content:  "GET https://example.com",
			expected: []Assertion{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAssertions(tt.content)

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
