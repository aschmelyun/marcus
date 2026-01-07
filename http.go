package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// runTest executes a single test and validates its assertions
func runTest(test Test) error {
	// Apply retry defaults
	retryDelay := test.RetryDelay
	if retryDelay == 0 {
		retryDelay = 1 * time.Second
	}
	retryMax := test.RetryMax
	if retryMax == 0 {
		retryMax = 10
	}

	// Prepare the request body content (needed for potential retries)
	var bodyContent string
	if test.Body != "" {
		if test.ContentType == "application/x-www-form-urlencoded" {
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
			bodyContent = formData.Encode()
		} else {
			bodyContent = test.Body
		}
	}

	client := &http.Client{}
	var lastStatusCode int
	var attempt int

	for {
		attempt++

		// Create fresh request for each attempt
		var bodyReader io.Reader
		if bodyContent != "" {
			bodyReader = strings.NewReader(bodyContent)
		}

		req, err := http.NewRequest(test.Method, test.URL, bodyReader)
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

		// Execute request and measure duration
		start := time.Now()
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("request failed: %w", err)
		}

		// Read response body
		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		duration := time.Since(start)
		if err != nil {
			return fmt.Errorf("failed to read response: %w", err)
		}

		lastStatusCode = resp.StatusCode

		// If waiting for a specific status and we haven't got it yet
		if test.WaitForStatus != 0 && resp.StatusCode != test.WaitForStatus {
			if attempt >= retryMax {
				return fmt.Errorf("wait for status %d failed: got %d after %d attempts", test.WaitForStatus, lastStatusCode, attempt)
			}
			time.Sleep(retryDelay)
			continue
		}

		// Parse response as JSON for field assertions
		var respJSON map[string]interface{}
		json.Unmarshal(respBody, &respJSON) // Ignore error - might not be JSON

		// Validate assertions
		for _, assertion := range test.Assertions {
			if err := validateAssertion(assertion, resp.StatusCode, respBody, respJSON, duration); err != nil {
				return err
			}
		}

		return nil
	}
}

// validateAssertion checks a single assertion against the response
func validateAssertion(assertion Assertion, statusCode int, body []byte, jsonBody map[string]interface{}, duration time.Duration) error {
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

	case "duration":
		maxDuration, err := parseDuration(assertion.Value)
		if err != nil {
			return fmt.Errorf("invalid duration in assertion: %s", assertion.Value)
		}
		if duration > maxDuration {
			return fmt.Errorf("duration assertion failed: expected < %s, got %s", formatDuration(maxDuration), formatDuration(duration))
		}

	case "body_matches_file":
		expectedContent, err := os.ReadFile(assertion.Value)
		if err != nil {
			return fmt.Errorf("body matches file assertion failed: could not read file '%s': %w", assertion.Value, err)
		}
		// Normalize JSON for comparison (re-marshal both to handle formatting differences)
		var expectedJSON, actualJSON interface{}
		if err := json.Unmarshal(expectedContent, &expectedJSON); err != nil {
			// Not JSON, do exact string comparison
			if string(body) != string(expectedContent) {
				return fmt.Errorf("body matches file assertion failed: response does not match file '%s'", assertion.Value)
			}
		} else {
			if err := json.Unmarshal(body, &actualJSON); err != nil {
				return fmt.Errorf("body matches file assertion failed: response is not valid JSON")
			}
			expectedNorm, _ := json.Marshal(expectedJSON)
			actualNorm, _ := json.Marshal(actualJSON)
			if string(expectedNorm) != string(actualNorm) {
				return fmt.Errorf("body matches file assertion failed: response does not match file '%s'", assertion.Value)
			}
		}
	}

	return nil
}

// parseDuration parses a duration string like "500ms" or "2s"
func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	return time.ParseDuration(s)
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
