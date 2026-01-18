package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: marcus [--parallel] [--quiet] [--only=N] [--skip=N] [--start-from=N] <file-or-directory>")
		os.Exit(1)
	}

	// Parse arguments
	parallel := false
	quiet := false
	only := 0      // 0 means run all tests
	skip := 0      // 0 means skip none
	startFrom := 0 // 0 means start from beginning
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
		} else if len(arg) > 7 && arg[:7] == "--skip=" {
			var n int
			_, err := fmt.Sscanf(arg, "--skip=%d", &n)
			if err != nil || n < 1 {
				fmt.Fprintln(os.Stderr, "Error: --skip requires a positive integer (e.g., --skip=3)")
				os.Exit(1)
			}
			skip = n
		} else if len(arg) > 13 && arg[:13] == "--start-from=" {
			var n int
			_, err := fmt.Sscanf(arg, "--start-from=%d", &n)
			if err != nil || n < 1 {
				fmt.Fprintln(os.Stderr, "Error: --start-from requires a positive integer (e.g., --start-from=3)")
				os.Exit(1)
			}
			startFrom = n
		} else if target == "" {
			target = arg
		}
	}

	if target == "" {
		fmt.Fprintln(os.Stderr, "Usage: marcus [--parallel] [--quiet] [--only=N] [--skip=N] [--start-from=N] <file-or-directory>")
		os.Exit(1)
	}

	if only > 0 && skip > 0 {
		fmt.Fprintln(os.Stderr, "Error: --only and --skip cannot be used together")
		os.Exit(1)
	}

	if only > 0 && startFrom > 0 {
		fmt.Fprintln(os.Stderr, "Error: --only and --start-from cannot be used together")
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

	// Skip a single test if --skip is specified
	if skip > 0 {
		if skip > totalTests {
			fmt.Fprintf(os.Stderr, "Error: test %d does not exist (file has %d tests)\n", skip, totalTests)
			os.Exit(1)
		}
		// Remove the test at position 'skip' (1-indexed)
		testNum := 0
		for i, tf := range testFiles {
			for j := range tf.Tests {
				testNum++
				if testNum == skip {
					// Remove test j from this file
					testFiles[i].Tests = append(tf.Tests[:j], tf.Tests[j+1:]...)
					// Remove the file if it has no tests left
					if len(testFiles[i].Tests) == 0 {
						testFiles = append(testFiles[:i], testFiles[i+1:]...)
					}
					goto skipped
				}
			}
		}
	skipped:
		totalTests--
	}

	// Start from a specific test if --start-from is specified
	if startFrom > 0 {
		if startFrom > totalTests {
			fmt.Fprintf(os.Stderr, "Error: test %d does not exist (file has %d tests)\n", startFrom, totalTests)
			os.Exit(1)
		}
		// Remove all tests before 'startFrom' (1-indexed)
		testsToSkip := startFrom - 1
		skipped := 0
		for i := 0; i < len(testFiles) && skipped < testsToSkip; {
			tf := &testFiles[i]
			testsInFile := len(tf.Tests)
			if skipped+testsInFile <= testsToSkip {
				// Skip entire file
				skipped += testsInFile
				testFiles = append(testFiles[:i], testFiles[i+1:]...)
			} else {
				// Skip some tests from this file
				toSkipHere := testsToSkip - skipped
				tf.Tests = tf.Tests[toSkipHere:]
				skipped += toSkipHere
				i++
			}
		}
		totalTests = totalTests - testsToSkip
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
