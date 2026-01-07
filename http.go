package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

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
