package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

var version = "1.0.0"

func main() {
	// Reorder args so flags after positional args still work.
	// Go's flag package stops parsing at the first non-flag argument,
	// so "diskbench /tmp --speed" would not parse --speed.
	reorderArgs()

	// CLI flags
	listFlag := flag.Bool("list", false, "List detected disks and exit")
	healthFlag := flag.Bool("health", false, "Run health check only")
	speedFlag := flag.Bool("speed", false, "Run speed test only")
	iopsFlag := flag.Bool("iops", false, "Run IOPS test only")
	allFlag := flag.Bool("all", false, "Run all tests (default if none selected)")
	sizeFlag := flag.String("size", "", "Test file size (e.g., 256M, 1G, 4G). Default: auto")
	durationFlag := flag.Int("duration", 10, "IOPS test duration in seconds")
	syncFlag := flag.Bool("sync", false, "Fsync after each IOPS write (measures real disk, not cache)")
	noColorFlag := flag.Bool("no-color", false, "Disable colored output")
	versionFlag := flag.Bool("version", false, "Show version and exit")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: diskbench [options] [target]\n\n")
		fmt.Fprintf(os.Stderr, "DiskBench - Cross-platform disk health, speed & IOPS tester\n\n")
		fmt.Fprintf(os.Stderr, "Arguments:\n")
		fmt.Fprintf(os.Stderr, "  target    Path or device to test (e.g., /dev/sda, /tmp, D:\\, /mnt/nfs)\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  diskbench --list              List detected disks\n")
		fmt.Fprintf(os.Stderr, "  diskbench /tmp --speed        Speed test on /tmp\n")
		fmt.Fprintf(os.Stderr, "  diskbench /dev/sda --health   Health check on /dev/sda\n")
		fmt.Fprintf(os.Stderr, "  diskbench --all --size 1G     All tests, 1GB test file\n")
		fmt.Fprintf(os.Stderr, "  diskbench /tmp --iops --sync  IOPS with fsync (real disk perf)\n")
	}

	flag.Parse()

	if *versionFlag {
		fmt.Printf("diskbench v%s\n", version)
		return
	}

	// Signal handling: clean up test files on interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintf(os.Stdout, "\n  Interrupted. Cleaning up...\n")
		cleanupAll()
		os.Exit(130)
	}()

	// Initialize colors
	initColors(*noColorFlag)

	// Print banner
	printHeader()
	printSystemInfo()
	fmt.Println()

	// Target
	target := ""
	if flag.NArg() > 0 {
		target = flag.Arg(0)
	}

	// List mode
	if *listFlag {
		disks := detectDisks()
		if len(disks) == 0 {
			fmt.Println("  No disks detected.")
		} else {
			printDiskList(disks)
		}
		return
	}

	// Determine which tests to run
	runHealth := *healthFlag
	runSpeed := *speedFlag
	runIOPS := *iopsFlag
	if *allFlag || (!runHealth && !runSpeed && !runIOPS) {
		runHealth = true
		runSpeed = true
		runIOPS = true
	}

	// Resolve disks
	var disks []DiskInfo
	if target != "" {
		disks = resolveTarget(target)
	} else {
		disks = detectDisks()
		if len(disks) == 0 {
			fmt.Println("  No disks detected.")
			return
		}
		printDiskList(disks)
		fmt.Println()
	}

	// Test each disk
	for _, disk := range disks {
		printDiskSectionHeader(disk)
		fmt.Println()

		// Health check
		if runHealth {
			result := checkHealth(disk)
			printHealthReport(result)
			fmt.Println()
		}

		// Determine test directory
		testDir := disk.MountPoint
		if testDir == "" || !isDir(testDir) {
			if runSpeed || runIOPS {
				fmt.Fprintf(os.Stdout, "  %sWarning: No writable mount point for %s, skipping benchmarks.%s\n",
					colorYellow, disk.Device, colorReset)
			}
			continue
		}

		// Check write permission
		if !isWritable(testDir) {
			if runSpeed || runIOPS {
				fmt.Fprintf(os.Stdout, "  %sWarning: %s is not writable, skipping benchmarks.%s\n",
					colorYellow, testDir, colorReset)
			}
			continue
		}

		// Speed test
		if runSpeed {
			testSize := int64(0)
			if *sizeFlag != "" {
				testSize = parseSize(*sizeFlag)
			}
			if testSize <= 0 {
				testSize = autoTestSize(disk)
			}

			// Check available space
			testSize = checkAvailableSpace(testDir, testSize)

			result := speedTest(testDir, testSize, defaultBlockSize)
			fmt.Println()
			printSpeedReport(result, disk.DiskType)
			fmt.Println()
		}

		// IOPS test
		if runIOPS {
			results := iopsTest(testDir, *durationFlag, *syncFlag)
			fmt.Println()
			if len(results) > 0 {
				printIOPSReport(results, disk.DiskType)
			}
			fmt.Println()
		}
	}

	fmt.Println("  Done.")
}

// reorderArgs moves flags before positional args so flag.Parse() sees them.
func reorderArgs() {
	var flags, positional []string
	args := os.Args[1:]
	skip := false
	for i, a := range args {
		if skip {
			flags = append(flags, a)
			skip = false
			continue
		}
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			// Check if this flag takes a value (e.g., --size 1G, --duration 10)
			if strings.Contains(a, "=") {
				continue // value is embedded: --size=1G
			}
			// Peek at next arg to see if it's a value
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				// Could be a flag value; check known value-flags
				base := strings.TrimLeft(a, "-")
				switch base {
				case "size", "duration":
					skip = true
				}
			}
		} else {
			positional = append(positional, a)
		}
	}
	os.Args = append([]string{os.Args[0]}, append(flags, positional...)...)
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isWritable(path string) bool {
	testFile := path + "/.diskbench_write_test"
	f, err := os.Create(testFile)
	if err != nil {
		return false
	}
	f.Close()
	os.Remove(testFile)
	return true
}

func checkAvailableSpace(dir string, requestedSize int64) int64 {
	size := getPartitionSize(dir)
	if size <= 0 {
		return requestedSize
	}
	// Use at most 50% of available space, minimum 64MB
	maxSize := size / 2
	if requestedSize > maxSize {
		requestedSize = maxSize
	}
	minSize := int64(64 * 1024 * 1024)
	if requestedSize < minSize {
		requestedSize = minSize
	}
	return requestedSize
}
