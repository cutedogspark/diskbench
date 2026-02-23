package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// detectDisks returns all detected disks on the current platform.
func detectDisks() []DiskInfo {
	return detectDisksPlatform() // Implemented per-platform
}

// resolveTarget resolves a user-specified path/device to DiskInfo objects.
func resolveTarget(target string) []DiskInfo {
	// 1. If directory: findDeviceForPath -> matchToPhysical -> return
	// 2. If block device (/dev/*, \\.\*): lookup in detectDisks
	// 3. If file exists: use parent directory
	// 4. Else: error

	info, err := os.Stat(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: '%s' not found\n", target)
		os.Exit(1)
	}

	if info.IsDir() {
		device := findDeviceForPath(target)
		matched := matchDeviceToPhysical(device)
		if matched != nil {
			matched.MountPoint = target
			return []DiskInfo{*matched}
		}
		// Create minimal DiskInfo
		dtype := guessDiskType(device)
		size := getPartitionSize(target)
		name := target
		if b := filepath.Base(target); b != "." && b != "/" {
			name = b
		}
		return []DiskInfo{{
			Device: device, Name: name, DiskType: dtype,
			Interface: "Unknown", SizeBytes: size, MountPoint: target,
		}}
	}

	// Block device
	if strings.HasPrefix(target, "/dev/") || strings.HasPrefix(target, `\\.\`) {
		disks := detectDisks()
		for _, d := range disks {
			if d.Device == target {
				return []DiskInfo{d}
			}
		}
		return []DiskInfo{{Device: target, Name: target, DiskType: "unknown",
			Interface: "Unknown", MountPoint: ""}}
	}

	// File: use parent directory
	return resolveTarget(filepath.Dir(target))
}

// findDeviceForPath uses 'df' to find the block device for a path.
func findDeviceForPath(path string) string {
	out, err := runCmd("df", path)
	if err != nil {
		return path
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) >= 2 {
		fields := strings.Fields(lines[1])
		if len(fields) > 0 {
			return fields[0]
		}
	}
	return path
}

// matchDeviceToPhysical tries to match a partition/volume device to its physical disk.
func matchDeviceToPhysical(device string) *DiskInfo {
	if device == "" {
		return nil
	}
	disks := detectDisks()

	// Platform-specific matching
	matched := matchDevicePlatform(device, disks)
	if matched != nil {
		return matched
	}

	// Generic: check if device starts with a known physical disk device
	devBase := filepath.Base(device)
	for _, d := range disks {
		physBase := filepath.Base(d.Device)
		if strings.HasPrefix(devBase, physBase) && devBase != physBase {
			copy := d
			copy.Device = device
			copy.MountPoint = ""
			return &copy
		}
	}
	return nil
}

// guessDiskType makes a best guess from a device path string.
func guessDiskType(device string) string {
	dev := strings.ToLower(device)
	if strings.Contains(dev, "nfs") || strings.Contains(dev, ":") {
		return "nfs"
	}
	if strings.Contains(dev, "nvme") {
		return "nvme"
	}
	if strings.Contains(dev, "usb") {
		return "usb"
	}
	return "ssd"
}

// getPartitionSize returns the total size of the filesystem at path.
func getPartitionSize(path string) int64 {
	// Implemented per-platform via getPartitionSizePlatform
	return getPartitionSizePlatform(path)
}

// autoTestSize determines the test file size based on disk capacity.
func autoTestSize(disk DiskInfo) int64 {
	if disk.DiskType == "nfs" {
		return 512 * 1024 * 1024 // 512MB
	}
	size := disk.SizeBytes
	switch {
	case size > 0 && size < 32*1024*1024*1024:
		return 256 * 1024 * 1024 // 256MB
	case size >= 32*1024*1024*1024 && size < 512*1024*1024*1024:
		return 1024 * 1024 * 1024 // 1GB
	case size >= 512*1024*1024*1024:
		return 4 * 1024 * 1024 * 1024 // 4GB
	default:
		return 1024 * 1024 * 1024 // 1GB default
	}
}
