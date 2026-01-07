package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Test represents a single API test parsed from markdown
type Test struct {
	Name        string
	Method      string
	URL         string
	Headers     map[string]string
	Body        string
	ContentType string
	Assertions  []Assertion
}

// Assertion represents a single assertion to validate
type Assertion struct {
	Type  string // "status", "body_contains", "field_equals"
	Field string // for field_equals: the field path (e.g., "json.username")
	Value string // expected value
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: marcus <markdown-file>")
		os.Exit(1)
	}

	filename := os.Args[1]

	content, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	tests := parseTests(string(content))

	if len(tests) == 0 {
		fmt.Println("No tests found in the markdown file.")
		return
	}

	fmt.Printf("Found %d test(s) to run\n\n", len(tests))

	for i, test := range tests {
		fmt.Printf("[%d/%d] %s\n", i+1, len(tests), test.Name)
		fmt.Printf("       %s %s\n", test.Method, test.URL)

		if err := runTest(test); err != nil {
			fmt.Fprintf(os.Stderr, "       FAIL: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("       PASS\n")
	}

	fmt.Printf("\nAll %d test(s) passed!\n", len(tests))
}

// parseTests extracts all tests from markdown content
func parseTests(content string) []Test {
	var tests []Test

	// Split by ## headers to get individual test blocks
	testPattern := regexp.MustCompile(`(?m)^## (.+)$`)
	matches := testPattern.FindAllStringSubmatchIndex(content, -1)

	for i, match := range matches {
		nameStart, nameEnd := match[2], match[3]
		testName := content[nameStart:nameEnd]

		// Get the content of this test block (until next ## or end)
		blockStart := match[1]
		blockEnd := len(content)
		if i+1 < len(matches) {
			blockEnd = matches[i+1][0]
		}
		blockContent := content[blockStart:blockEnd]

		test := parseTestBlock(testName, blockContent)
		if test.URL != "" {
			tests = append(tests, test)
		}
	}

	return tests
}

// parseTestBlock parses a single test block
func parseTestBlock(name, content string) Test {
	test := Test{
		Name:    name,
		Method:  "GET",
		Headers: make(map[string]string),
	}

	lines := strings.Split(content, "\n")

	// Find the HTTP method and URL line
	httpPattern := regexp.MustCompile(`^(GET|POST|PUT|PATCH|DELETE)\s+(https?://\S+)`)
	var methodLineIdx int

	for i, line := range lines {
		if matches := httpPattern.FindStringSubmatch(line); matches != nil {
			test.Method = matches[1]
			test.URL = matches[2]
			methodLineIdx = i
			break
		}
	}

	if test.URL == "" {
		return test
	}

	// Parse headers (bullet points starting with "- " right after the URL line)
	headerPattern := regexp.MustCompile(`^-\s+([^:]+):\s*(.+)$`)
	for i := methodLineIdx + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			continue
		}
		if matches := headerPattern.FindStringSubmatch(line); matches != nil {
			headerName := strings.TrimSpace(matches[1])
			headerValue := strings.TrimSpace(matches[2])
			test.Headers[headerName] = headerValue
			if strings.EqualFold(headerName, "Content-Type") {
				test.ContentType = headerValue
			}
		} else {
			break // Stop at first non-header line
		}
	}

	// Parse code blocks for body content
	codeBlockPattern := regexp.MustCompile("(?s)```(json|form)\\s*\n(.+?)```")
	if matches := codeBlockPattern.FindStringSubmatch(content); matches != nil {
		blockType := matches[1]
		blockContent := strings.TrimSpace(matches[2])

		if blockType == "json" {
			test.Body = blockContent
			if test.ContentType == "" {
				test.ContentType = "application/json"
			}
		} else if blockType == "form" {
			test.Body = blockContent
			if test.ContentType == "" {
				test.ContentType = "application/x-www-form-urlencoded"
			}
		}
	}

	// Parse assertions
	test.Assertions = parseAssertions(content)

	return test
}

// parseAssertions extracts assertions from a test block
func parseAssertions(content string) []Assertion {
	var assertions []Assertion

	// Find the assertions section (starts with "Assert:" or "Asserts:")
	assertPattern := regexp.MustCompile(`(?m)^Asserts?:\s*$`)
	loc := assertPattern.FindStringIndex(content)
	if loc == nil {
		return assertions
	}

	// Get content after "Assert(s):"
	assertContent := content[loc[1]:]

	// Parse each assertion line
	lines := strings.Split(assertContent, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "- ") {
			break // Stop at first non-bullet line
		}

		line = strings.TrimPrefix(line, "- ")

		// Status assertion: "Status is 200"
		if strings.HasPrefix(line, "Status is ") {
			value := strings.TrimPrefix(line, "Status is ")
			assertions = append(assertions, Assertion{
				Type:  "status",
				Value: value,
			})
			continue
		}

		// Body contains assertion: "Body contains `field`"
		bodyContainsPattern := regexp.MustCompile("^Body contains `([^`]+)`")
		if matches := bodyContainsPattern.FindStringSubmatch(line); matches != nil {
			assertions = append(assertions, Assertion{
				Type:  "body_contains",
				Field: matches[1],
			})
			continue
		}

		// Field equals assertion: "Field `path` equals `value`"
		fieldEqualsPattern := regexp.MustCompile("^Field `([^`]+)` equals `([^`]+)`")
		if matches := fieldEqualsPattern.FindStringSubmatch(line); matches != nil {
			assertions = append(assertions, Assertion{
				Type:  "field_equals",
				Field: matches[1],
				Value: matches[2],
			})
			continue
		}
	}

	return assertions
}

// runTest executes a single test and validates its assertions
func runTest(test Test) error {
	var req *http.Request
	var err error

	// Prepare the request body
	var bodyReader io.Reader
	if test.Body != "" {
		if test.ContentType == "application/x-www-form-urlencoded" {
			// Convert form body to URL-encoded format
			formData := url.Values{}
			for _, line := range strings.Split(test.Body, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					formData.Set(parts[0], parts[1])
				}
			}
			bodyReader = strings.NewReader(formData.Encode())
		} else {
			bodyReader = strings.NewReader(test.Body)
		}
	}

	req, err = http.NewRequest(test.Method, test.URL, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	for key, value := range test.Headers {
		req.Header.Set(key, value)
	}
	if test.ContentType != "" {
		req.Header.Set("Content-Type", test.ContentType)
	}

	// Execute request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Parse response as JSON for field assertions
	var respJSON map[string]interface{}
	json.Unmarshal(respBody, &respJSON) // Ignore error - might not be JSON

	// Validate assertions
	for _, assertion := range test.Assertions {
		if err := validateAssertion(assertion, resp.StatusCode, respBody, respJSON); err != nil {
			return err
		}
	}

	return nil
}

// validateAssertion checks a single assertion against the response
func validateAssertion(assertion Assertion, statusCode int, body []byte, jsonBody map[string]interface{}) error {
	switch assertion.Type {
	case "status":
		expected, err := strconv.Atoi(assertion.Value)
		if err != nil {
			return fmt.Errorf("invalid status code in assertion: %s", assertion.Value)
		}
		if statusCode != expected {
			return fmt.Errorf("status assertion failed: expected %d, got %d", expected, statusCode)
		}

	case "body_contains":
		if jsonBody == nil {
			return fmt.Errorf("body contains assertion failed: response is not valid JSON")
		}
		if _, exists := jsonBody[assertion.Field]; !exists {
			return fmt.Errorf("body contains assertion failed: field '%s' not found in response", assertion.Field)
		}

	case "field_equals":
		if jsonBody == nil {
			return fmt.Errorf("field equals assertion failed: response is not valid JSON")
		}
		actual, err := getJSONField(jsonBody, assertion.Field)
		if err != nil {
			return fmt.Errorf("field equals assertion failed: %w", err)
		}

		// Parse the expected value (handle quoted strings, booleans, numbers)
		expected := parseExpectedValue(assertion.Value)

		if !valuesEqual(actual, expected) {
			return fmt.Errorf("field equals assertion failed: field '%s' expected %v, got %v", assertion.Field, expected, actual)
		}
	}

	return nil
}

// getJSONField retrieves a nested field from JSON using dot notation
func getJSONField(data map[string]interface{}, path string) (interface{}, error) {
	parts := strings.Split(path, ".")
	var current interface{} = data

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			var exists bool
			current, exists = v[part]
			if !exists {
				return nil, fmt.Errorf("field '%s' not found", path)
			}
		default:
			return nil, fmt.Errorf("cannot traverse into non-object at '%s'", part)
		}
	}

	return current, nil
}

// parseExpectedValue converts an assertion value string to the appropriate type
func parseExpectedValue(value string) interface{} {
	// Handle quoted strings: "value" -> value
	if strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
		return strings.Trim(value, `"`)
	}

	// Handle booleans
	if value == "true" {
		return true
	}
	if value == "false" {
		return false
	}

	// Handle numbers
	if i, err := strconv.ParseInt(value, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(value, 64); err == nil {
		return f
	}

	// Default to string
	return value
}

// valuesEqual compares two values for equality, handling type conversions
func valuesEqual(actual, expected interface{}) bool {
	// Direct equality
	if actual == expected {
		return true
	}

	// String comparison (JSON often returns strings)
	actualStr := fmt.Sprintf("%v", actual)
	expectedStr := fmt.Sprintf("%v", expected)

	return actualStr == expectedStr
}
