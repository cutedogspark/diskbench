package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

// ANSI color codes.
var (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorRed    = "\033[91m"
	colorGreen  = "\033[92m"
	colorYellow = "\033[93m"
	colorBlue   = "\033[94m"
	colorCyan   = "\033[96m"
	colorDim    = "\033[2m"
	useUnicode  = true
)

// Box-drawing characters (Unicode defaults, ASCII fallback).
var (
	boxTL = "┌"
	boxTR = "┐"
	boxBL = "└"
	boxBR = "┘"
	boxH  = "─"
	boxV  = "│"
	boxML = "├"
	boxMR = "┤"
	boxMT = "┬"
	boxMB = "┴"
	boxMM = "┼"
)

// initColors disables colours when noColor is true or stdout is not a terminal.
// It also sets the box-drawing characters to ASCII when Unicode is unavailable.
func initColors(noColor bool) {
	if noColor || !isTerminal() {
		colorReset = ""
		colorBold = ""
		colorRed = ""
		colorGreen = ""
		colorYellow = ""
		colorBlue = ""
		colorCyan = ""
		colorDim = ""
		useUnicode = false
	}

	// Check for a known-limited TERM that cannot render Unicode.
	term := os.Getenv("TERM")
	if term == "dumb" || term == "linux" {
		useUnicode = false
	}

	if !useUnicode {
		boxTL = "+"
		boxTR = "+"
		boxBL = "+"
		boxBR = "+"
		boxH = "-"
		boxV = "|"
		boxML = "+"
		boxMR = "+"
		boxMT = "+"
		boxMB = "+"
		boxMM = "+"
	}
}

// isTerminal reports whether stdout is connected to a terminal.
func isTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// ---------------------------------------------------------------------------
// Rating system
// ---------------------------------------------------------------------------

type ratingThreshold struct {
	threshold float64
	label     string
}

var speedRatings = map[string][]ratingThreshold{
	"nvme": {{2000, "Excellent"}, {1000, "Good"}, {500, "Fair"}, {0, "Slow"}},
	"ssd":  {{500, "Excellent"}, {300, "Good"}, {100, "Fair"}, {0, "Slow"}},
	"hdd":  {{150, "Excellent"}, {100, "Good"}, {50, "Fair"}, {0, "Slow"}},
	"usb":  {{300, "Excellent"}, {100, "Good"}, {30, "Fair"}, {0, "Slow"}},
	"nfs":  {{500, "Excellent"}, {100, "Good"}, {50, "Fair"}, {0, "Slow"}},
}

var iopsRatings = map[string][]ratingThreshold{
	"nvme": {{100000, "Excellent"}, {50000, "Good"}, {10000, "Fair"}, {0, "Slow"}},
	"ssd":  {{50000, "Excellent"}, {10000, "Good"}, {1000, "Fair"}, {0, "Slow"}},
	"hdd":  {{200, "Excellent"}, {100, "Good"}, {50, "Fair"}, {0, "Slow"}},
	"usb":  {{5000, "Excellent"}, {1000, "Good"}, {100, "Fair"}, {0, "Slow"}},
	"nfs":  {{10000, "Excellent"}, {1000, "Good"}, {100, "Fair"}, {0, "Slow"}},
}

func rateSpeed(mbps float64, diskType string) string {
	thresholds, ok := speedRatings[strings.ToLower(diskType)]
	if !ok {
		thresholds = speedRatings["ssd"]
	}
	for _, t := range thresholds {
		if mbps >= t.threshold {
			return t.label
		}
	}
	return "Slow"
}

func rateIOPS(iops float64, diskType string) string {
	thresholds, ok := iopsRatings[strings.ToLower(diskType)]
	if !ok {
		thresholds = iopsRatings["ssd"]
	}
	for _, t := range thresholds {
		if iops >= t.threshold {
			return t.label
		}
	}
	return "Slow"
}

func ratingColor(rating string) string {
	switch rating {
	case "Excellent":
		return colorGreen
	case "Good":
		return colorBlue
	case "Fair":
		return colorYellow
	case "Slow":
		return colorRed
	default:
		return colorDim
	}
}

// ---------------------------------------------------------------------------
// Table printer
// ---------------------------------------------------------------------------

// printTable renders a bordered table to stdout.
// aligns is a slice of alignment chars: 'l' left, 'r' right, 'c' center.
func printTable(headers []string, rows [][]string, aligns []byte) {
	cols := len(headers)
	if cols == 0 {
		return
	}

	// Compute column widths (content + 2 padding).
	widths := make([]int, cols)
	for i, h := range headers {
		if len(h) > widths[i] {
			widths[i] = len(h)
		}
	}
	for _, row := range rows {
		for i := 0; i < cols && i < len(row); i++ {
			vis := visibleLen(row[i])
			if vis > widths[i] {
				widths[i] = vis
			}
		}
	}
	// Add 2 for padding on each side.
	for i := range widths {
		widths[i] += 2
	}

	// Helper: build a horizontal line like ┌──┬──┐
	hline := func(left, mid, right string) string {
		var sb strings.Builder
		sb.WriteString(left)
		for i, w := range widths {
			sb.WriteString(strings.Repeat(boxH, w))
			if i < cols-1 {
				sb.WriteString(mid)
			}
		}
		sb.WriteString(right)
		return sb.String()
	}

	// Helper: format a cell.
	cell := func(text string, width int, align byte) string {
		// width includes the 2 padding chars; inner width is width-2.
		inner := width - 2
		vis := visibleLen(text)
		pad := inner - vis
		if pad < 0 {
			pad = 0
		}
		switch align {
		case 'r':
			return " " + strings.Repeat(" ", pad) + text + " "
		case 'c':
			left := pad / 2
			right := pad - left
			return " " + strings.Repeat(" ", left) + text + strings.Repeat(" ", right) + " "
		default: // 'l'
			return " " + text + strings.Repeat(" ", pad) + " "
		}
	}

	// Helper: build a data row.
	dataRow := func(values []string, bold bool) string {
		var sb strings.Builder
		sb.WriteString(boxV)
		for i := 0; i < cols; i++ {
			v := ""
			if i < len(values) {
				v = values[i]
			}
			a := byte('l')
			if i < len(aligns) {
				a = aligns[i]
			}
			content := cell(v, widths[i], a)
			if bold {
				content = colorBold + content + colorReset
			}
			sb.WriteString(content)
			if i < cols-1 {
				sb.WriteString(boxV)
			}
		}
		sb.WriteString(boxV)
		return sb.String()
	}

	fmt.Println(hline(boxTL, boxMT, boxTR))
	fmt.Println(dataRow(headers, true))
	fmt.Println(hline(boxML, boxMM, boxMR))
	for _, row := range rows {
		fmt.Println(dataRow(row, false))
	}
	fmt.Println(hline(boxBL, boxMB, boxBR))
}

// visibleLen returns the visible length of s, ignoring ANSI escape sequences.
func visibleLen(s string) int {
	n := 0
	inEsc := false
	for _, r := range s {
		if r == '\033' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		n++
	}
	return n
}

// ---------------------------------------------------------------------------
// Progress bar
// ---------------------------------------------------------------------------

func progressBar(fraction float64, width int) string {
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}
	inner := width - 2 // account for [ and ]
	if inner < 1 {
		inner = 1
	}
	filled := int(fraction * float64(inner))
	empty := inner - filled

	fillChar := "#"
	if useUnicode {
		fillChar = "\u2588" // █
	}

	pct := int(fraction * 100)
	return "[" + strings.Repeat(fillChar, filled) + strings.Repeat(" ", empty) + "] " + fmt.Sprintf("%d%%", pct)
}

// ---------------------------------------------------------------------------
// Output helpers
// ---------------------------------------------------------------------------

func printHeader() {
	fmt.Println()
	title := fmt.Sprintf("DiskBench v%s", version)
	sub := "Cross-platform disk health, speed & IOPS tester"
	fmt.Printf("  %s%s%s\n", colorBold, title, colorReset)
	fmt.Printf("  %s%s%s\n", colorDim, sub, colorReset)
	fmt.Println()
}

func printSystemInfo() {
	info := fmt.Sprintf("System: %s (%s) | Go %s",
		runtime.GOOS, runtime.GOARCH, runtime.Version())
	fmt.Printf("  %s%s%s\n\n", colorDim, info, colorReset)
}

func printDiskList(disks []DiskInfo) {
	headers := []string{"Device", "Name", "Type", "Interface", "Size", "Mount"}
	aligns := []byte{'l', 'l', 'c', 'l', 'r', 'l'}
	rows := make([][]string, len(disks))
	for i, d := range disks {
		rows[i] = []string{
			d.Device,
			d.Name,
			strings.ToUpper(d.DiskType),
			d.Interface,
			formatSize(d.SizeBytes),
			d.MountPoint,
		}
	}
	printTable(headers, rows, aligns)
}

func printDiskSectionHeader(disk DiskInfo) {
	sep := "━━"
	if !useUnicode {
		sep = "=="
	}
	label := fmt.Sprintf("%s %s (%s) %s - %s %s %s",
		sep, disk.Device, disk.Name,
		strings.ToUpper(disk.DiskType), disk.Interface,
		formatSize(disk.SizeBytes), sep)
	fmt.Println()
	fmt.Printf("  %s%s%s\n", colorBold, label, colorReset)
	fmt.Println()
}

func printHealthReport(result HealthResult) {
	// Status line.
	var statusColor string
	switch result.Status {
	case "HEALTHY":
		statusColor = colorGreen
	case "WARNING":
		statusColor = colorYellow
	case "CRITICAL":
		statusColor = colorRed
	default:
		statusColor = colorDim
	}
	fmt.Printf("  Health: %s%s%s\n", statusColor, result.Status, colorReset)

	if result.Message != "" {
		fmt.Printf("  %s%s%s\n", colorDim, result.Message, colorReset)
	}

	if len(result.Attributes) > 0 {
		fmt.Println()
		headers := []string{"Attribute", "Value", "Status"}
		aligns := []byte{'l', 'r', 'c'}
		rows := make([][]string, len(result.Attributes))
		for i, a := range result.Attributes {
			statusStr := a.Status
			switch a.Status {
			case "OK":
				statusStr = colorGreen + a.Status + colorReset
			case "WARN":
				statusStr = colorYellow + a.Status + colorReset
			case "FAIL":
				statusStr = colorRed + a.Status + colorReset
			}
			rows[i] = []string{a.Name, a.Value, statusStr}
		}
		printTable(headers, rows, aligns)
	}
	fmt.Println()
}

func printSpeedReport(result SpeedResult, diskType string) {
	if result.DirectIO {
		fmt.Printf("  %sNote: using direct I/O (bypassing OS cache)%s\n", colorDim, colorReset)
	} else {
		fmt.Printf("  %sNote: using buffered I/O (direct I/O not available)%s\n", colorDim, colorReset)
	}
	fmt.Printf("  %sTest size: %s | Block size: %s%s\n",
		colorDim, formatSize(result.TestSize), formatSize(int64(result.BlockSize)), colorReset)
	fmt.Println()

	readRating := rateSpeed(result.ReadMBPS, diskType)
	writeRating := rateSpeed(result.WriteMBPS, diskType)

	headers := []string{"Test", "Speed (MB/s)", "Rating"}
	aligns := []byte{'l', 'r', 'c'}
	rows := [][]string{
		{
			"Sequential Read",
			formatFloat(result.ReadMBPS, 1),
			ratingColor(readRating) + readRating + colorReset,
		},
		{
			"Sequential Write",
			formatFloat(result.WriteMBPS, 1),
			ratingColor(writeRating) + writeRating + colorReset,
		},
	}
	printTable(headers, rows, aligns)
	fmt.Println()
}

func printIOPSReport(results []IOPSResult, diskType string) {
	fmt.Println()

	headers := []string{"Test", "IOPS", "Latency (us)", "Rating"}
	aligns := []byte{'l', 'r', 'r', 'c'}
	var rows [][]string
	for _, r := range results {
		readRating := rateIOPS(r.ReadIOPS, diskType)
		writeRating := rateIOPS(r.WriteIOPS, diskType)
		rows = append(rows, []string{
			r.Label + " Read",
			formatFloat(r.ReadIOPS, 0),
			formatFloat(r.ReadLatencyUS, 1),
			ratingColor(readRating) + readRating + colorReset,
		})
		rows = append(rows, []string{
			r.Label + " Write",
			formatFloat(r.WriteIOPS, 0),
			formatFloat(r.WriteLatencyUS, 1),
			ratingColor(writeRating) + writeRating + colorReset,
		})
	}
	printTable(headers, rows, aligns)
	fmt.Println()
}
