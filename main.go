package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
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

// TestFile represents a markdown file containing tests
type TestFile struct {
	Path  string
	Tests []Test
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

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: marcus [--parallel] <file-or-directory>")
		os.Exit(1)
	}

	// Parse arguments
	parallel := false
	target := ""

	for _, arg := range os.Args[1:] {
		if arg == "--parallel" {
			parallel = true
		} else if target == "" {
			target = arg
		}
	}

	if target == "" {
		fmt.Fprintln(os.Stderr, "Usage: marcus [--parallel] <file-or-directory>")
		os.Exit(1)
	}

	testFiles, err := collectTestFiles(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(testFiles) == 0 {
		fmt.Println("No test files found.")
		return
	}

	// Count total tests across all files
	totalTests := 0
	for _, tf := range testFiles {
		totalTests += len(tf.Tests)
	}

	if totalTests == 0 {
		fmt.Println("No tests found.")
		return
	}

	// Print summary header
	if len(testFiles) == 1 {
		fmt.Printf("%s (%d tests)\n\n", testFiles[0].Path, totalTests)
	} else {
		fmt.Printf("%s (%d files, %d tests)\n\n", target, len(testFiles), totalTests)
	}

	var passed, failed int
	var totalDuration time.Duration

	if parallel {
		passed, failed, totalDuration = runTestsParallel(testFiles)
	} else {
		passed, failed, totalDuration = runTestsSequential(testFiles)
	}

	if failed == 0 {
		fmt.Printf("%d passed in %s\n", passed, formatDuration(totalDuration))
	} else {
		fmt.Printf("%d passed, %d failed in %s\n", passed, failed, formatDuration(totalDuration))
		os.Exit(1)
	}
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}

// runTestsSequential runs all tests one after another
func runTestsSequential(testFiles []TestFile) (passed, failed int, totalDuration time.Duration) {
	suiteStart := time.Now()

	for _, tf := range testFiles {
		fileStart := time.Now()

		if len(testFiles) > 1 {
			fmt.Printf("%s\n", tf.Path)
		}

		for _, test := range tf.Tests {
			if err := runTest(test); err != nil {
				fmt.Printf("  ✗ %s\n", test.Name)
				fmt.Printf("    → %v\n", err)
				failed++
			} else {
				fmt.Printf("  ✓ %s\n", test.Name)
				passed++
			}
		}

		fileDuration := time.Since(fileStart)
		if len(testFiles) > 1 {
			fmt.Printf("  %s\n\n", formatDuration(fileDuration))
		}
	}

	if len(testFiles) == 1 {
		fmt.Println()
	}

	totalDuration = time.Since(suiteStart)
	return passed, failed, totalDuration
}

// runTestsParallel runs all tests concurrently, limited by CPU cores
func runTestsParallel(testFiles []TestFile) (passed, failed int, totalDuration time.Duration) {
	suiteStart := time.Now()
	maxWorkers := runtime.NumCPU()
	sem := make(chan struct{}, maxWorkers)

	// Build flat list of all tests with their file context
	type testJob struct {
		filePath  string
		fileIndex int
		testIndex int
		test      Test
	}

	var jobs []testJob
	for fi, tf := range testFiles {
		for ti, test := range tf.Tests {
			jobs = append(jobs, testJob{
				filePath:  tf.Path,
				fileIndex: fi,
				testIndex: ti,
				test:      test,
			})
		}
	}

	// Results slice
	results := make([]TestResult, len(jobs))
	var wg sync.WaitGroup

	for i, job := range jobs {
		wg.Add(1)
		go func(idx int, j testJob) {
			defer wg.Done()
			sem <- struct{}{}        // Acquire
			defer func() { <-sem }() // Release

			start := time.Now()
			err := runTest(j.test)
			results[idx] = TestResult{
				FilePath:  j.filePath,
				FileIndex: j.fileIndex,
				Test:      j.test,
				Index:     idx,
				Err:       err,
				Duration:  time.Since(start),
			}
		}(i, job)
	}

	wg.Wait()

	// Calculate per-file durations (sum of test durations in that file)
	fileDurations := make(map[int]time.Duration)
	for _, result := range results {
		fileDurations[result.FileIndex] += result.Duration
	}

	// Print results in order, grouped by file
	currentFile := ""
	currentFileIndex := -1
	for i, job := range jobs {
		if len(testFiles) > 1 && job.filePath != currentFile {
			// Print previous file's duration
			if currentFile != "" {
				fmt.Printf("  %s\n\n", formatDuration(fileDurations[currentFileIndex]))
			}
			fmt.Printf("%s\n", job.filePath)
			currentFile = job.filePath
			currentFileIndex = job.fileIndex
		}

		result := results[i]
		if result.Err != nil {
			fmt.Printf("  ✗ %s\n", result.Test.Name)
			fmt.Printf("    → %v\n", result.Err)
			failed++
		} else {
			fmt.Printf("  ✓ %s\n", result.Test.Name)
			passed++
		}
	}

	// Print last file's duration if multiple files
	if len(testFiles) > 1 {
		fmt.Printf("  %s\n\n", formatDuration(fileDurations[currentFileIndex]))
	} else {
		fmt.Println()
	}

	totalDuration = time.Since(suiteStart)
	return passed, failed, totalDuration
}

// collectTestFiles gathers all test files from a file or directory path
func collectTestFiles(path string) ([]TestFile, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	var testFiles []TestFile

	if info.IsDir() {
		// Walk directory recursively for .md files
		err := filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && strings.HasSuffix(p, ".md") {
				content, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				tests := parseTests(string(content))
				if len(tests) > 0 {
					testFiles = append(testFiles, TestFile{Path: p, Tests: tests})
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		// Sort by path for consistent ordering
		sort.Slice(testFiles, func(i, j int) bool {
			return testFiles[i].Path < testFiles[j].Path
		})
	} else {
		// Single file
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		tests := parseTests(string(content))
		if len(tests) > 0 {
			testFiles = append(testFiles, TestFile{Path: path, Tests: tests})
		}
	}

	return testFiles, nil
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
