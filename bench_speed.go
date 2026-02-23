package main

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	cleanupMu    sync.Mutex
	cleanupFiles []string
)

func registerCleanup(path string) {
	cleanupMu.Lock()
	cleanupFiles = append(cleanupFiles, path)
	cleanupMu.Unlock()
}

func unregisterCleanup(path string) {
	cleanupMu.Lock()
	for i, f := range cleanupFiles {
		if f == path {
			cleanupFiles = append(cleanupFiles[:i], cleanupFiles[i+1:]...)
			break
		}
	}
	cleanupMu.Unlock()
}

func cleanupAll() {
	cleanupMu.Lock()
	for _, f := range cleanupFiles {
		os.Remove(f)
	}
	cleanupFiles = nil
	cleanupMu.Unlock()
}

const defaultBlockSize = 1024 * 1024 // 1MB

func speedTest(testDir string, totalSize int64, blockSize int) SpeedResult {
	if blockSize <= 0 {
		blockSize = defaultBlockSize
	}

	testFile := filepath.Join(testDir, ".diskbench_speed")
	registerCleanup(testFile)
	defer func() {
		os.Remove(testFile)
		unregisterCleanup(testFile)
	}()

	numBlocks := int(totalSize) / blockSize
	if numBlocks < 1 {
		numBlocks = 1
	}

	// Pre-generate random data block
	dataBlock := make([]byte, blockSize)
	rand.Read(dataBlock)

	result := SpeedResult{
		TestSize:  totalSize,
		BlockSize: blockSize,
	}

	// === WRITE TEST ===
	writeFile, directWrite := openDirectWrite(testFile)
	if writeFile == nil {
		// Fallback to normal open
		var err error
		writeFile, err = os.OpenFile(testFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Error creating test file: %v\n", err)
			return result
		}
		setNoCache(writeFile)
	}

	start := time.Now()
	for i := 0; i < numBlocks; i++ {
		_, err := writeFile.Write(dataBlock)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Write error at block %d: %v\n", i, err)
			break
		}
		// Progress bar
		frac := float64(i+1) / float64(numBlocks)
		speed := float64((i+1)*blockSize) / time.Since(start).Seconds() / (1024 * 1024)
		fmt.Fprintf(os.Stdout, "\r  Sequential Write:  %s  %s MB/s", progressBar(frac, 24), formatFloat(speed, 1))
	}
	writeFile.Sync()
	writeElapsed := time.Since(start)
	writeFile.Close()

	result.WriteMBPS = float64(totalSize) / writeElapsed.Seconds() / (1024 * 1024)
	fmt.Fprintf(os.Stdout, "\r  Sequential Write:  %s  %s MB/s\n", progressBar(1.0, 24), formatFloat(result.WriteMBPS, 1))

	// === DROP CACHES ===
	dropCaches()

	// === READ TEST ===
	readFile, directRead := openDirectRead(testFile)
	if readFile == nil {
		var err error
		readFile, err = os.Open(testFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Error opening test file for read: %v\n", err)
			return result
		}
	}
	result.DirectIO = directWrite || directRead

	readBuf := alignedBuffer(blockSize)
	start = time.Now()
	totalRead := int64(0)
	for totalRead < totalSize {
		n, err := readFile.Read(readBuf)
		if err != nil && err != io.EOF {
			break
		}
		if n == 0 {
			break
		}
		totalRead += int64(n)

		frac := float64(totalRead) / float64(totalSize)
		speed := float64(totalRead) / time.Since(start).Seconds() / (1024 * 1024)
		fmt.Fprintf(os.Stdout, "\r  Sequential Read:   %s  %s MB/s", progressBar(frac, 24), formatFloat(speed, 1))
	}
	readElapsed := time.Since(start)
	readFile.Close()

	if totalRead > 0 {
		result.ReadMBPS = float64(totalRead) / readElapsed.Seconds() / (1024 * 1024)
	}
	fmt.Fprintf(os.Stdout, "\r  Sequential Read:   %s  %s MB/s\n", progressBar(1.0, 24), formatFloat(result.ReadMBPS, 1))

	return result
}
