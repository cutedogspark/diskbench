package main

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

const (
	iopsBlockSize = 4096 // 4K
)

// autoIOPSFileSize returns an appropriate IOPS test file size based on disk type.
// The file must be larger than the device/controller cache to avoid measuring
// cache performance instead of actual disk performance.
//
//	NVMe:  2 GB  (DRAM cache typically 512MB-1GB)
//	SSD:   1 GB  (DRAM cache typically 256MB-512MB)
//	HDD:   256 MB (on-disk cache 8-256MB)
//	USB:   128 MB (little or no cache)
//	NFS:   256 MB (network, no local cache concern)
//	RAID:  4 GB  (hardware RAID controllers often have 1-4GB write-back cache)
func autoIOPSFileSize(disk DiskInfo) int64 {
	const (
		_128MB = 128 * 1024 * 1024
		_256MB = 256 * 1024 * 1024
		_1GB   = 1024 * 1024 * 1024
		_2GB   = 2 * 1024 * 1024 * 1024
		_4GB   = 4 * 1024 * 1024 * 1024
	)

	// RAID controller detected — use large file to bypass controller cache
	if disk.Interface == "RAID" {
		return _4GB
	}

	switch disk.DiskType {
	case "nvme":
		return _2GB
	case "ssd":
		return _1GB
	case "hdd":
		return _256MB
	case "usb":
		return _128MB
	case "nfs":
		return _256MB
	default:
		return _1GB
	}
}

func iopsTest(testDir string, duration int, useSync bool, disk DiskInfo) []IOPSResult {
	if duration <= 0 {
		duration = 10
	}

	testFile := filepath.Join(testDir, ".diskbench_iops")
	registerCleanup(testFile)
	defer func() {
		os.Remove(testFile)
		unregisterCleanup(testFile)
	}()

	fileSize := checkAvailableSpace(testDir, autoIOPSFileSize(disk))

	// Create test file
	fmt.Fprintf(os.Stdout, "  Preparing IOPS test file (%s)...", formatSize(fileSize))
	if err := createTestFile(testFile, fileSize); err != nil {
		fmt.Fprintf(os.Stdout, " error: %v\n", err)
		return nil
	}
	fmt.Fprintf(os.Stdout, " done.\n")
	if useSync {
		fmt.Fprintf(os.Stdout, "  %sNote: --sync enabled, fsync after each write (measures real disk)%s\n", colorYellow, colorReset)
	}

	numPositions := int64(fileSize / iopsBlockSize)
	var results []IOPSResult

	// QD1 Write
	writeIOPS, writeLat := iopsWriteQD1(testFile, numPositions, duration, useSync)
	fmt.Fprintf(os.Stdout, "  Random Write QD1: %10s IOPS\n", formatNumber(int64(writeIOPS)))

	// QD1 Read
	readIOPS, readLat := iopsReadQD1(testFile, numPositions, duration)
	fmt.Fprintf(os.Stdout, "  Random Read  QD1: %10s IOPS\n", formatNumber(int64(readIOPS)))

	results = append(results, IOPSResult{
		Label: "QD1", ReadIOPS: readIOPS, WriteIOPS: writeIOPS,
		ReadLatencyUS: readLat, WriteLatencyUS: writeLat,
		QueueDepth: 1, BlockSize: iopsBlockSize, Duration: float64(duration),
	})

	// QD4 Write
	writeIOPS4, writeLat4 := iopsWriteQD(testFile, numPositions, duration, 4, useSync)
	fmt.Fprintf(os.Stdout, "  Random Write QD4: %10s IOPS\n", formatNumber(int64(writeIOPS4)))

	// QD4 Read
	readIOPS4, readLat4 := iopsReadQD(testFile, numPositions, duration, 4)
	fmt.Fprintf(os.Stdout, "  Random Read  QD4: %10s IOPS\n", formatNumber(int64(readIOPS4)))

	results = append(results, IOPSResult{
		Label: "QD4", ReadIOPS: readIOPS4, WriteIOPS: writeIOPS4,
		ReadLatencyUS: readLat4, WriteLatencyUS: writeLat4,
		QueueDepth: 4, BlockSize: iopsBlockSize, Duration: float64(duration),
	})

	return results
}

func createTestFile(path string, size int64) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	buf := make([]byte, 1024*1024) // 1MB chunks
	rand.Read(buf)

	written := int64(0)
	for written < size {
		n, err := f.Write(buf)
		if err != nil {
			return err
		}
		written += int64(n)
	}
	return f.Sync()
}

func randomOffset(numPositions int64) int64 {
	n, _ := rand.Int(rand.Reader, big.NewInt(numPositions))
	return n.Int64() * iopsBlockSize
}

func iopsWriteQD1(path string, numPositions int64, duration int, useSync bool) (iops float64, latencyUS float64) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return 0, 0
	}
	defer f.Close()

	data := make([]byte, iopsBlockSize)
	rand.Read(data)

	ops := int64(0)
	totalLat := float64(0)
	deadline := time.Now().Add(time.Duration(duration) * time.Second)

	for time.Now().Before(deadline) {
		offset := randomOffset(numPositions)
		t0 := time.Now()
		f.WriteAt(data, offset)
		if useSync {
			f.Sync()
		}
		totalLat += time.Since(t0).Seconds()
		ops++
	}

	elapsed := float64(duration)
	iops = float64(ops) / elapsed
	if ops > 0 {
		latencyUS = (totalLat / float64(ops)) * 1e6
	}
	return
}

func iopsReadQD1(path string, numPositions int64, duration int) (iops float64, latencyUS float64) {
	f, _ := openDirectRead(path)
	if f == nil {
		var err error
		f, err = os.Open(path)
		if err != nil {
			return 0, 0
		}
	}
	defer f.Close()

	buf := alignedBuffer(iopsBlockSize)
	ops := int64(0)
	totalLat := float64(0)
	deadline := time.Now().Add(time.Duration(duration) * time.Second)

	for time.Now().Before(deadline) {
		offset := randomOffset(numPositions)
		t0 := time.Now()
		f.ReadAt(buf, offset)
		totalLat += time.Since(t0).Seconds()
		ops++
	}

	elapsed := float64(duration)
	iops = float64(ops) / elapsed
	if ops > 0 {
		latencyUS = (totalLat / float64(ops)) * 1e6
	}
	return
}

func iopsWriteQD(path string, numPositions int64, duration, qd int, useSync bool) (iops float64, latencyUS float64) {
	var totalOps int64
	var totalLat int64 // nanoseconds, atomic
	var wg sync.WaitGroup

	deadline := time.Now().Add(time.Duration(duration) * time.Second)

	for i := 0; i < qd; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f, err := os.OpenFile(path, os.O_RDWR, 0)
			if err != nil {
				return
			}
			defer f.Close()

			data := make([]byte, iopsBlockSize)
			rand.Read(data)

			localOps := int64(0)
			localLat := int64(0)

			for time.Now().Before(deadline) {
				offset := randomOffset(numPositions)
				t0 := time.Now()
				f.WriteAt(data, offset)
				if useSync {
					f.Sync()
				}
				localLat += time.Since(t0).Nanoseconds()
				localOps++
			}

			atomic.AddInt64(&totalOps, localOps)
			atomic.AddInt64(&totalLat, localLat)
		}()
	}
	wg.Wait()

	elapsed := float64(duration)
	iops = float64(totalOps) / elapsed
	if totalOps > 0 {
		latencyUS = float64(totalLat) / float64(totalOps) / 1000
	}
	return
}

func iopsReadQD(path string, numPositions int64, duration, qd int) (iops float64, latencyUS float64) {
	var totalOps int64
	var totalLat int64
	var wg sync.WaitGroup

	deadline := time.Now().Add(time.Duration(duration) * time.Second)

	for i := 0; i < qd; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			f, _ := openDirectRead(path)
			if f == nil {
				var err error
				f, err = os.Open(path)
				if err != nil {
					return
				}
			}
			defer f.Close()

			buf := alignedBuffer(iopsBlockSize)
			localOps := int64(0)
			localLat := int64(0)

			for time.Now().Before(deadline) {
				offset := randomOffset(numPositions)
				t0 := time.Now()
				f.ReadAt(buf, offset)
				localLat += time.Since(t0).Nanoseconds()
				localOps++
			}

			atomic.AddInt64(&totalOps, localOps)
			atomic.AddInt64(&totalLat, localLat)
		}()
	}
	wg.Wait()

	elapsed := float64(duration)
	iops = float64(totalOps) / elapsed
	if totalOps > 0 {
		latencyUS = float64(totalLat) / float64(totalOps) / 1000
	}
	return
}
