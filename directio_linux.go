//go:build linux

package main

import (
	"os"
	"os/exec"
	"syscall"
	"time"
	"unsafe"
)

func openDirectRead(path string) (*os.File, bool) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_DIRECT, 0)
	if err != nil {
		return nil, false
	}
	return os.NewFile(uintptr(fd), path), true
}

func openDirectWrite(path string) (*os.File, bool) {
	fd, err := syscall.Open(path, syscall.O_WRONLY|syscall.O_CREAT|syscall.O_TRUNC|syscall.O_DIRECT, 0644)
	if err != nil {
		return nil, false
	}
	return os.NewFile(uintptr(fd), path), true
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
	_ = f // unused; Linux uses O_DIRECT instead
	return false
}

func dropCaches() {
	_ = exec.Command("sync").Run()
	_ = os.WriteFile("/proc/sys/vm/drop_caches", []byte("3\n"), 0644)
	time.Sleep(500 * time.Millisecond)
}
