package main

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// runCmd executes a command and returns its stdout as a string.
func runCmd(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.Output()
	return string(out), err
}

// runCmdRaw executes a command and returns stdout bytes (for plist/binary data).
func runCmdRaw(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Output()
}

// runCmdTimeout executes with a custom timeout.
func runCmdTimeout(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.Output()
	return string(out), err
}

// findExecutable returns the full path of an executable, or empty string.
func findExecutable(name string) string {
	p, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return p
}

// parseSize converts a human size string (e.g. "256M", "1G") to bytes.
func parseSize(s string) int64 {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return 0
	}
	multipliers := map[byte]int64{
		'K': 1024,
		'M': 1024 * 1024,
		'G': 1024 * 1024 * 1024,
		'T': 1024 * 1024 * 1024 * 1024,
	}
	last := s[len(s)-1]
	if m, ok := multipliers[last]; ok {
		val, err := strconv.ParseFloat(s[:len(s)-1], 64)
		if err != nil {
			return 0
		}
		return int64(val * float64(m))
	}
	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return val
}

// formatSize formats bytes into human-readable form (e.g. "465.9 GB").
func formatSize(bytes int64) string {
	if bytes <= 0 {
		return "-"
	}
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	val := float64(bytes)
	for _, u := range units {
		if math.Abs(val) < 1024 || u == "PB" {
			if val == math.Trunc(val) && val < 10000 {
				return fmt.Sprintf("%.0f %s", val, u)
			}
			return fmt.Sprintf("%.1f %s", val, u)
		}
		val /= 1024
	}
	return fmt.Sprintf("%.1f PB", val)
}

// formatNumber adds thousand separators (e.g. 1234567 -> "1,234,567").
func formatNumber(n int64) string {
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, byte(c))
	}
	return string(result)
}

// formatFloat formats a float with thousand separators.
func formatFloat(f float64, decimals int) string {
	intPart := int64(f)
	formatted := formatNumber(intPart)
	if decimals > 0 {
		frac := f - float64(intPart)
		if frac < 0 {
			frac = -frac
		}
		fracStr := strconv.FormatFloat(frac, 'f', decimals, 64)
		// fracStr is like "0.5", we want ".5"
		if len(fracStr) > 1 {
			formatted += fracStr[1:]
		}
	}
	return formatted
}

// parseLsblkSize parses lsblk size strings like "500G", "1.8T".
func parseLsblkSize(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	last := s[len(s)-1]
	if !unicode.IsDigit(rune(last)) {
		return parseSize(s)
	}
	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return val
}
