package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: marcus [--parallel] [--quiet] [--only=N] <file-or-directory>")
		os.Exit(1)
	}

	// Parse arguments
	parallel := false
	quiet := false
	only := 0 // 0 means run all tests
	target := ""

	for _, arg := range os.Args[1:] {
		if arg == "--parallel" {
			parallel = true
		} else if arg == "--quiet" || arg == "-q" {
			quiet = true
		} else if len(arg) > 7 && arg[:7] == "--only=" {
			var n int
			_, err := fmt.Sscanf(arg, "--only=%d", &n)
			if err != nil || n < 1 {
				fmt.Fprintln(os.Stderr, "Error: --only requires a positive integer (e.g., --only=3)")
				os.Exit(1)
			}
			only = n
		} else if target == "" {
			target = arg
		}
	}

	if target == "" {
		fmt.Fprintln(os.Stderr, "Usage: marcus [--parallel] [--quiet] [--only=N] <file-or-directory>")
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

	// Filter to single test if --only is specified
	if only > 0 {
		if only > totalTests {
			fmt.Fprintf(os.Stderr, "Error: test %d does not exist (file has %d tests)\n", only, totalTests)
			os.Exit(1)
		}
		// Find the test at position 'only' (1-indexed)
		testNum := 0
		for i, tf := range testFiles {
			for j, test := range tf.Tests {
				testNum++
				if testNum == only {
					testFiles = []TestFile{{Path: tf.Path, Tests: []Test{test}}}
					testFiles[0].Tests[0].Name = fmt.Sprintf("%s (#%d)", test.Name, only)
					_ = i // silence unused warning
					_ = j
					goto filtered
				}
			}
		}
	filtered:
		totalTests = 1
	}

	// Print summary header
	if !quiet {
		if len(testFiles) == 1 {
			fmt.Printf("%s (%d tests)\n\n", testFiles[0].Path, totalTests)
		} else {
			fmt.Printf("%s (%d files, %d tests)\n\n", target, len(testFiles), totalTests)
		}
	}

	var passed, failed int
	var totalDuration time.Duration

	if parallel {
		passed, failed, totalDuration = runTestsParallel(testFiles, quiet)
	} else {
		passed, failed, totalDuration = runTestsSequential(testFiles, quiet)
	}

	if failed == 0 {
		fmt.Printf("%s%d passed%s %sin %s%s\n", colorGreen, passed, colorReset, colorDim, formatDuration(totalDuration), colorReset)
	} else {
		fmt.Printf("%s%d passed%s, %s%d failed%s %sin %s%s\n", colorGreen, passed, colorReset, colorRed, failed, colorReset, colorDim, formatDuration(totalDuration), colorReset)
		os.Exit(1)
	}
}
