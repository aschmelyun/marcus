package main

import (
	"regexp"
	"strings"
)

// parseTests extracts all tests from markdown content
func parseTests(content string) []Test {
	var tests []Test

	// Parse frontmatter for defaults
	defaults, content := parseFrontmatter(content)

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

		test := parseTestBlock(testName, blockContent, defaults)
		if test.URL != "" {
			tests = append(tests, test)
		}
	}

	return tests
}

// parseFrontmatter extracts YAML frontmatter from content
func parseFrontmatter(content string) (Defaults, string) {
	defaults := Defaults{
		Headers: make(map[string]string),
	}

	// Check if content starts with frontmatter delimiter
	if !strings.HasPrefix(strings.TrimSpace(content), "---") {
		return defaults, content
	}

	// Find the closing delimiter
	content = strings.TrimSpace(content)
	lines := strings.Split(content, "\n")

	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "---" {
		return defaults, content
	}

	// Find closing ---
	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			endIdx = i
			break
		}
	}

	if endIdx == -1 {
		return defaults, content
	}

	// Parse the frontmatter content
	inHeaders := false
	for i := 1; i < endIdx; i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			continue
		}

		// Check for "headers:" section
		if trimmed == "headers:" {
			inHeaders = true
			continue
		}

		// Parse header entries (indented lines under headers:)
		if inHeaders && (strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "\t")) {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				defaults.Headers[key] = value
			}
		} else {
			// No longer in headers section
			inHeaders = false
		}
	}

	// Return remaining content after frontmatter
	remaining := strings.Join(lines[endIdx+1:], "\n")
	return defaults, remaining
}

// parseTestBlock parses a single test block
func parseTestBlock(name, content string, defaults Defaults) Test {
	test := Test{
		Name:    name,
		Method:  "GET",
		Headers: make(map[string]string),
	}

	// Apply default headers first
	for key, value := range defaults.Headers {
		test.Headers[key] = value
		if strings.EqualFold(key, "Content-Type") {
			test.ContentType = value
		}
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
	// These override any defaults
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

		// Duration assertion: "Duration < 500ms" or "Time < 2s"
		durationPattern := regexp.MustCompile("^(?:Duration|Time) < (.+)$")
		if matches := durationPattern.FindStringSubmatch(line); matches != nil {
			assertions = append(assertions, Assertion{
				Type:  "duration",
				Value: matches[1],
			})
			continue
		}
	}

	return assertions
}
