package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorDim    = "\033[2m"
	colorBold   = "\033[1m"
)

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
				fmt.Printf("  %s✗%s %s\n", colorRed, colorReset, test.Name)
				fmt.Printf("    %s→ %v%s\n", colorRed, err, colorReset)
				failed++
			} else {
				fmt.Printf("  %s✓%s %s\n", colorGreen, colorReset, test.Name)
				passed++
			}
		}

		fileDuration := time.Since(fileStart)
		if len(testFiles) > 1 {
			fmt.Printf("  %s%s%s\n\n", colorDim, formatDuration(fileDuration), colorReset)
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

	// Calculate per-file durations (max test duration since they run in parallel)
	fileDurations := make(map[int]time.Duration)
	for _, result := range results {
		if result.Duration > fileDurations[result.FileIndex] {
			fileDurations[result.FileIndex] = result.Duration
		}
	}

	// Print results in order, grouped by file
	currentFile := ""
	currentFileIndex := -1
	for i, job := range jobs {
		if len(testFiles) > 1 && job.filePath != currentFile {
			// Print previous file's duration
			if currentFile != "" {
				fmt.Printf("  %s%s%s\n\n", colorDim, formatDuration(fileDurations[currentFileIndex]), colorReset)
			}
			fmt.Printf("%s\n", job.filePath)
			currentFile = job.filePath
			currentFileIndex = job.fileIndex
		}

		result := results[i]
		if result.Err != nil {
			fmt.Printf("  %s✗%s %s\n", colorRed, colorReset, result.Test.Name)
			fmt.Printf("    %s→ %v%s\n", colorRed, result.Err, colorReset)
			failed++
		} else {
			fmt.Printf("  %s✓%s %s\n", colorGreen, colorReset, result.Test.Name)
			passed++
		}
	}

	// Print last file's duration if multiple files
	if len(testFiles) > 1 {
		fmt.Printf("  %s%s%s\n\n", colorDim, formatDuration(fileDurations[currentFileIndex]), colorReset)
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
				baseDir := filepath.Dir(p)
				tests := parseTests(string(content), baseDir)
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
		baseDir := filepath.Dir(path)
		tests := parseTests(string(content), baseDir)
		if len(tests) > 0 {
			testFiles = append(testFiles, TestFile{Path: path, Tests: tests})
		}
	}

	return testFiles, nil
}
