package main

import (
	"encoding/base64"
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

// interpolateVariables replaces {{variable}} placeholders with saved values
func interpolateVariables(s string, vars map[string]interface{}) string {
	if vars == nil {
		return s
	}
	result := s
	for name, value := range vars {
		placeholder := "{{" + name + "}}"
		result = strings.ReplaceAll(result, placeholder, fmt.Sprintf("%v", value))
	}
	return result
}

// runTest executes a single test and validates its assertions
// vars contains saved variables from previous tests, and returns updated variables
func runTest(test Test, vars map[string]interface{}) (map[string]interface{}, error) {
	if vars == nil {
		vars = make(map[string]interface{})
	}

	// Interpolate variables in URL, headers, and body
	test.URL = interpolateVariables(test.URL, vars)
	test.Body = interpolateVariables(test.Body, vars)
	for key, value := range test.Headers {
		test.Headers[key] = interpolateVariables(value, vars)
	}
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
			return vars, fmt.Errorf("failed to create request: %w", err)
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
			return vars, fmt.Errorf("request failed: %w", err)
		}

		// Read response body
		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		duration := time.Since(start)
		if err != nil {
			return vars, fmt.Errorf("failed to read response: %w", err)
		}

		lastStatusCode = resp.StatusCode

		// If waiting for a specific status and we haven't got it yet
		if test.WaitForStatus != 0 && resp.StatusCode != test.WaitForStatus {
			if attempt >= retryMax {
				return vars, fmt.Errorf("wait for status %d failed: got %d after %d attempts", test.WaitForStatus, lastStatusCode, attempt)
			}
			time.Sleep(retryDelay)
			continue
		}

		// Parse response as JSON for field assertions
		var respJSON map[string]interface{}
		json.Unmarshal(respBody, &respJSON) // Ignore error - might not be JSON

		// If waiting for a specific field value and we haven't got it yet
		if test.WaitForField != "" {
			actual, err := getJSONField(respJSON, test.WaitForField)
			expected := parseExpectedValue(test.WaitForValue)
			if err != nil || !valuesEqual(actual, expected) {
				if attempt >= retryMax {
					if err != nil {
						return vars, fmt.Errorf("wait for field `%s` failed: field not found after %d attempts", test.WaitForField, attempt)
					}
					return vars, fmt.Errorf("wait for field `%s` equals `%s` failed: got `%v` after %d attempts", test.WaitForField, test.WaitForValue, actual, attempt)
				}
				time.Sleep(retryDelay)
				continue
			}
		}

		// Validate assertions
		for _, assertion := range test.Assertions {
			if err := validateAssertion(assertion, resp.StatusCode, respBody, respJSON, duration); err != nil {
				return vars, err
			}
		}

		// Save fields for use in subsequent tests
		for _, sf := range test.SaveFields {
			value, err := getJSONField(respJSON, sf.Field)
			if err != nil {
				return vars, fmt.Errorf("save field failed: %w", err)
			}
			vars[sf.Variable] = value
		}

		return vars, nil
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
			// Include response body in error for debugging (truncate if too long)
			bodyPreview := string(body)
			if len(bodyPreview) > 500 {
				bodyPreview = bodyPreview[:500] + "..."
			}
			if bodyPreview != "" {
				return fmt.Errorf("status assertion failed: expected %d, got %d\n       Response: %s", expected, statusCode, bodyPreview)
			}
			return fmt.Errorf("status assertion failed: expected %d, got %d", expected, statusCode)
		}

	case "body_contains":
		if jsonBody == nil {
			return fmt.Errorf("body contains assertion failed: response is not valid JSON")
		}
		fieldPath, transforms := splitFieldTransforms(assertion.Field)
		if len(transforms) > 0 {
			value, err := getJSONField(jsonBody, fieldPath)
			if err != nil {
				return fmt.Errorf("body contains assertion failed: field '%s' not found in response", fieldPath)
			}
			transformed, err := applyTransforms(fmt.Sprintf("%v", value), transforms)
			if err != nil {
				return fmt.Errorf("body contains assertion failed: %w", err)
			}
			if transformed == "" {
				return fmt.Errorf("body contains assertion failed: field '%s' is empty after transform", fieldPath)
			}
		} else {
			if _, exists := jsonBody[fieldPath]; !exists {
				return fmt.Errorf("body contains assertion failed: field '%s' not found in response", fieldPath)
			}
		}

	case "field_equals":
		if jsonBody == nil {
			return fmt.Errorf("field equals assertion failed: response is not valid JSON")
		}
		fieldPath, transforms := splitFieldTransforms(assertion.Field)
		actual, err := getJSONField(jsonBody, fieldPath)
		if err != nil {
			return fmt.Errorf("field equals assertion failed: %w", err)
		}

		if len(transforms) > 0 {
			transformed, err := applyTransforms(fmt.Sprintf("%v", actual), transforms)
			if err != nil {
				return fmt.Errorf("field equals assertion failed: %w", err)
			}
			expected := parseExpectedValue(assertion.Value)
			if !valuesEqual(transformed, expected) {
				return fmt.Errorf("field equals assertion failed: field '%s' expected %v, got %v (after transform)", assertion.Field, expected, transformed)
			}
		} else {
			expected := parseExpectedValue(assertion.Value)
			if !valuesEqual(actual, expected) {
				return fmt.Errorf("field equals assertion failed: field '%s' expected %v, got %v", assertion.Field, expected, actual)
			}
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

	case "body_partial_match":
		if jsonBody == nil {
			return fmt.Errorf("body partial match assertion failed: response is not valid JSON")
		}
		// Each line in Value is a JSON key-value pair to check
		// Format: "field": value  or  "field": "value"
		for _, line := range strings.Split(assertion.Value, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			// Remove trailing comma if present
			line = strings.TrimSuffix(line, ",")

			// Parse the line as a JSON key-value pair
			// Try wrapping in braces to parse as JSON object
			jsonLine := "{" + line + "}"
			var parsed map[string]interface{}
			if err := json.Unmarshal([]byte(jsonLine), &parsed); err != nil {
				return fmt.Errorf("body partial match assertion failed: invalid JSON line '%s': %w", line, err)
			}

			// Check each field in the parsed line against the response
			for field, expected := range parsed {
				actual, err := getJSONField(jsonBody, field)
				if err != nil {
					return fmt.Errorf("body partial match assertion failed: %w", err)
				}
				if !valuesEqual(actual, expected) {
					return fmt.Errorf("body partial match assertion failed: field '%s' expected %v, got %v", field, expected, actual)
				}
			}
		}
	}

	return nil
}

// splitFieldTransforms separates a field path from pipe-separated transforms.
// e.g. "data.token | base64" returns ("data.token", ["base64"])
func splitFieldTransforms(field string) (string, []string) {
	parts := strings.Split(field, "|")
	path := strings.TrimSpace(parts[0])
	var transforms []string
	for _, p := range parts[1:] {
		t := strings.TrimSpace(p)
		if t != "" {
			transforms = append(transforms, t)
		}
	}
	return path, transforms
}

// applyTransforms applies a sequence of named transforms to a string value.
// Currently supports: "base64" (base64 decode).
func applyTransforms(value string, transforms []string) (string, error) {
	result := value
	for _, t := range transforms {
		switch t {
		case "base64":
			// Try standard encoding first, then URL-safe, then raw variants
			decoded, err := base64.StdEncoding.DecodeString(result)
			if err != nil {
				decoded, err = base64.URLEncoding.DecodeString(result)
			}
			if err != nil {
				decoded, err = base64.RawStdEncoding.DecodeString(result)
			}
			if err != nil {
				decoded, err = base64.RawURLEncoding.DecodeString(result)
			}
			if err != nil {
				return "", fmt.Errorf("base64 decode failed for value %q: %w", result, err)
			}
			result = string(decoded)
		default:
			return "", fmt.Errorf("unknown transform: %s", t)
		}
	}
	return result, nil
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
