//go:build windows

package main

import (
	"os"
	"syscall"
	"time"
	"unsafe"
)

const (
	fileFlagNoBuffering    = 0x20000000
	fileFlagWriteThrough   = 0x80000000
	fileFlagSequentialScan = 0x08000000
)

func openDirectRead(path string) (*os.File, bool) {
	pathp, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, false
	}
	h, err := syscall.CreateFile(
		pathp,
		syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ,
		nil,
		syscall.OPEN_EXISTING,
		fileFlagNoBuffering|fileFlagSequentialScan,
		0)
	if err != nil {
		return nil, false
	}
	return os.NewFile(uintptr(h), path), true
}

func openDirectWrite(path string) (*os.File, bool) {
	pathp, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, false
	}
	h, err := syscall.CreateFile(
		pathp,
		syscall.GENERIC_WRITE,
		0,
		nil,
		syscall.CREATE_ALWAYS,
		fileFlagNoBuffering|fileFlagWriteThrough,
		0)
	if err != nil {
		return nil, false
	}
	return os.NewFile(uintptr(h), path), true
}

func alignedBuffer(size int) []byte {
	const align = 4096
	buf := make([]byte, size+align)
	addr := uintptr(unsafe.Pointer(&buf[0]))
	offset := int(align - (addr % uintptr(align)))
	if offset == align {
		offset = 0
	}
	return buf[offset : offset+size]
}

func setNoCache(f *os.File) bool {
	_ = f // unused; Windows uses CreateFile flags instead
	return false
}

func dropCaches() {
	time.Sleep(500 * time.Millisecond) // No easy way on Windows
}
