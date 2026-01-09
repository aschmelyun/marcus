package main

import "time"

// Test represents a single API test parsed from markdown
type Test struct {
	Name        string
	Method      string
	URL         string
	Headers     map[string]string
	Body        string
	ContentType string
	Assertions  []Assertion
	SaveFields  []SaveField // Fields to save for use in subsequent tests
	// Retry configuration for polling async endpoints
	WaitForStatus int           // Status code to wait for (0 = no waiting)
	WaitForField  string        // Field path to wait for (e.g., "message.code")
	WaitForValue  string        // Value the field should equal
	RetryDelay    time.Duration // Delay between retries (default: 1s)
	RetryMax      int           // Max retry attempts (default: 10)
}

// Assertion represents a single assertion to validate
type Assertion struct {
	Type  string // "status", "body_contains", "field_equals"
	Field string // for field_equals: the field path (e.g., "json.username")
	Value string // expected value
}

// SaveField represents a field to save from the response
type SaveField struct {
	Field    string // JSON path to extract (e.g., "data.id")
	Variable string // Variable name to save as (e.g., "user_id")
}

// TestFile represents a markdown file containing tests
type TestFile struct {
	Path  string
	Tests []Test
}

// Defaults holds default settings parsed from frontmatter
type Defaults struct {
	Root    string
	Headers map[string]string
}

// TestResult holds the outcome of a single test execution
type TestResult struct {
	FilePath  string
	FileIndex int
	Test      Test
	Index     int
	Err       error
	Duration  time.Duration
}
