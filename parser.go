package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// parseTests extracts all tests from markdown content
// baseDir is the directory containing the test file, used for resolving relative file paths
func parseTests(content string, baseDir string) []Test {
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

		test := parseTestBlock(testName, blockContent, defaults, baseDir)
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

		// Check for "root:" setting
		if strings.HasPrefix(trimmed, "root:") {
			defaults.Root = strings.TrimSpace(strings.TrimPrefix(trimmed, "root:"))
			// Remove trailing slash for consistent joining
			defaults.Root = strings.TrimSuffix(defaults.Root, "/")
			inHeaders = false
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
// baseDir is used for resolving relative file paths in FILE: references
func parseTestBlock(name, content string, defaults Defaults, baseDir string) Test {
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
	// Supports both absolute URLs (https://...) and relative paths (/path)
	httpPattern := regexp.MustCompile(`^(GET|POST|PUT|PATCH|DELETE)\s+(\S+)`)
	var methodLineIdx int

	for i, line := range lines {
		if matches := httpPattern.FindStringSubmatch(line); matches != nil {
			test.Method = matches[1]
			urlOrPath := matches[2]

			// If it's a relative path and we have a root, prepend the root
			if strings.HasPrefix(urlOrPath, "/") && defaults.Root != "" {
				test.URL = defaults.Root + urlOrPath
			} else if strings.HasPrefix(urlOrPath, "http://") || strings.HasPrefix(urlOrPath, "https://") {
				test.URL = urlOrPath
			} else if defaults.Root != "" {
				// Handle paths without leading slash
				test.URL = defaults.Root + "/" + urlOrPath
			} else {
				// No root and not an absolute URL - invalid but let it through for error handling
				test.URL = urlOrPath
			}

			methodLineIdx = i
			break
		}
	}

	if test.URL == "" {
		return test
	}

	// Parse headers and retry options (bullet points starting with "- " right after the URL line)
	// These override any defaults
	headerPattern := regexp.MustCompile(`^-\s+([^:]+):\s*(.+)$`)
	waitUntilPattern := regexp.MustCompile(`(?i)^-\s+Wait until status is (\d+)$`)
	retryPattern := regexp.MustCompile(`(?i)^-\s+Retry (\d+) times every (.+)$`)

	for i := methodLineIdx + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Check for natural language retry options first
		if matches := waitUntilPattern.FindStringSubmatch(line); matches != nil {
			if status, err := strconv.Atoi(matches[1]); err == nil {
				test.WaitForStatus = status
			}
			continue
		}

		if matches := retryPattern.FindStringSubmatch(line); matches != nil {
			if max, err := strconv.Atoi(matches[1]); err == nil {
				test.RetryMax = max
			}
			if d, err := time.ParseDuration(matches[2]); err == nil {
				test.RetryDelay = d
			}
			continue
		}

		// Parse as header
		if matches := headerPattern.FindStringSubmatch(line); matches != nil {
			optionName := strings.TrimSpace(matches[1])
			optionValue := strings.TrimSpace(matches[2])

			test.Headers[optionName] = optionValue
			if strings.EqualFold(optionName, "Content-Type") {
				test.ContentType = optionValue
			}
		} else {
			break // Stop at first non-header/option line
		}
	}

	// Parse code blocks for body content
	codeBlockPattern := regexp.MustCompile("(?s)```(json|form)\\s*\n(.+?)```")
	if matches := codeBlockPattern.FindStringSubmatch(content); matches != nil {
		blockType := matches[1]
		blockContent := strings.TrimSpace(matches[2])

		// Check if content is a file reference
		if strings.HasPrefix(blockContent, "FILE:") {
			filePath := strings.TrimSpace(strings.TrimPrefix(blockContent, "FILE:"))
			// Resolve relative path from test file's directory
			if !filepath.IsAbs(filePath) {
				filePath = filepath.Join(baseDir, filePath)
			}
			fileContent, err := os.ReadFile(filePath)
			if err == nil {
				blockContent = string(fileContent)
			}
			// If file can't be read, keep the FILE: reference as-is (will fail at runtime)
		}

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
	test.Assertions = parseAssertions(content, baseDir)

	return test
}

// parseAssertions extracts assertions from a test block
// baseDir is used for resolving relative file paths in FILE: references
func parseAssertions(content string, baseDir string) []Assertion {
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
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
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

		// Duration assertion: "Duration less than 500ms" or "Time less than 2s"
		durationPattern := regexp.MustCompile("^(?:Duration|Time) less than (.+)$")
		if matches := durationPattern.FindStringSubmatch(line); matches != nil {
			assertions = append(assertions, Assertion{
				Type:  "duration",
				Value: matches[1],
			})
			continue
		}

		// Body matches file assertion: "Body matches file `path/to/file.json`"
		bodyMatchesFilePattern := regexp.MustCompile("^Body matches file `([^`]+)`")
		if matches := bodyMatchesFilePattern.FindStringSubmatch(line); matches != nil {
			filePath := matches[1]
			// Resolve relative path from test file's directory
			if !filepath.IsAbs(filePath) {
				filePath = filepath.Join(baseDir, filePath)
			}
			assertions = append(assertions, Assertion{
				Type:  "body_matches_file",
				Value: filePath,
			})
			continue
		}

		// Body partially matches assertion: "Body partially matches:"
		// Followed by a code block where lines starting with >> are checked
		if line == "Body partially matches:" {
			// Look for the code block in the remaining content
			remainingContent := strings.Join(lines[i+1:], "\n")
			codeBlockPattern := regexp.MustCompile("(?s)^\\s*```(?:json)?\\s*\n(.+?)```")
			if matches := codeBlockPattern.FindStringSubmatch(remainingContent); matches != nil {
				blockContent := matches[1]
				// Extract lines marked with >> prefix
				var markedLines []string
				for _, blockLine := range strings.Split(blockContent, "\n") {
					trimmed := strings.TrimSpace(blockLine)
					if strings.HasPrefix(trimmed, ">>") {
						// Remove the >> prefix and any leading whitespace after it
						markedLine := strings.TrimSpace(strings.TrimPrefix(trimmed, ">>"))
						if markedLine != "" {
							markedLines = append(markedLines, markedLine)
						}
					}
				}
				if len(markedLines) > 0 {
					assertions = append(assertions, Assertion{
						Type:  "body_partial_match",
						Value: strings.Join(markedLines, "\n"),
					})
				}
				// Skip past the code block
				for j := i + 1; j < len(lines); j++ {
					if strings.Contains(lines[j], "```") {
						// Found opening, now find closing
						for k := j + 1; k < len(lines); k++ {
							if strings.Contains(lines[k], "```") {
								i = k
								break
							}
						}
						break
					}
				}
			}
			continue
		}
	}

	return assertions
}
