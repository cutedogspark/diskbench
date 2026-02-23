//go:build darwin

package main

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

func openDirectRead(path string) (*os.File, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	// F_NOCACHE = 48
	_, _, errno := syscall.Syscall(syscall.SYS_FCNTL, f.Fd(), 48, 1)
	ok := errno == 0
	return f, ok
}

func openDirectWrite(path string) (*os.File, bool) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return nil, false
	}
	_, _, errno := syscall.Syscall(syscall.SYS_FCNTL, f.Fd(), 48, 1)
	ok := errno == 0
	return f, ok
}

func alignedBuffer(size int) []byte {
	return make([]byte, size) // macOS F_NOCACHE doesn't require alignment
}

func setNoCache(f *os.File) bool {
	_, _, errno := syscall.Syscall(syscall.SYS_FCNTL, f.Fd(), 48, 1)
	return errno == 0
}

func dropCaches() {
	_ = exec.Command("sync").Run()
	_ = exec.Command("purge").Run()
	time.Sleep(500 * time.Millisecond)
}
