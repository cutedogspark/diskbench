//go:build !linux && !darwin && !windows

package main

import "os"

func openDirectRead(path string) (*os.File, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	return f, false // no direct I/O
}

func openDirectWrite(path string) (*os.File, bool) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return nil, false
	}
	return f, false
}

func alignedBuffer(size int) []byte {
	return make([]byte, size)
}

func setNoCache(f *os.File) bool {
	_ = f
	return false
}

func dropCaches() {}
