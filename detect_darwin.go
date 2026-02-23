//go:build darwin

package main

// Uses diskutil + plutil for disk detection on macOS

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

// plistToJSON converts plist data to JSON using plutil.
func plistToJSON(plistData []byte) ([]byte, error) {
	cmd := exec.Command("plutil", "-convert", "json", "-o", "-", "-")
	cmd.Stdin = strings.NewReader(string(plistData))
	return cmd.Output()
}

func detectDisksPlatform() []DiskInfo {
	var disks []DiskInfo

	// Step 1: Get all whole disks
	raw, err := runCmdRaw("diskutil", "list", "-plist")
	if err != nil {
		return disks
	}
	jsonData, err := plistToJSON(raw)
	if err != nil {
		return disks
	}

	var listResult map[string]interface{}
	if json.Unmarshal(jsonData, &listResult) != nil {
		return disks
	}

	wholeDisks, ok := listResult["WholeDisks"].([]interface{})
	if !ok {
		return disks
	}

	// Step 2: Get info for each whole disk
	for _, wd := range wholeDisks {
		diskName, ok := wd.(string)
		if !ok {
			continue
		}
		device := "/dev/" + diskName

		raw, err := runCmdRaw("diskutil", "info", "-plist", device)
		if err != nil {
			continue
		}
		jsonData, err := plistToJSON(raw)
		if err != nil {
			continue
		}

		var info map[string]interface{}
		if json.Unmarshal(jsonData, &info) != nil {
			continue
		}

		// Skip virtual disks
		if vp, ok := info["VirtualOrPhysical"].(string); ok && vp == "Virtual" {
			continue
		}
		// Require partition_scheme content for non-Physical disks
		vp, _ := info["VirtualOrPhysical"].(string)
		content, _ := info["Content"].(string)
		if vp != "Physical" && content != "" && !strings.Contains(content, "partition_scheme") {
			continue
		}

		bus, _ := info["BusProtocol"].(string)
		solidState, _ := info["SolidState"].(bool)
		internal, _ := info["Internal"].(bool)
		mediaName, _ := info["MediaName"].(string)
		if mediaName == "" {
			mediaName, _ = info["IORegistryEntryName"].(string)
		}
		if mediaName == "" {
			mediaName = diskName
		}

		totalSize := int64(0)
		if ts, ok := info["TotalSize"].(float64); ok {
			totalSize = int64(ts)
		} else if ts, ok := info["Size"].(float64); ok {
			totalSize = int64(ts)
		}

		mountPoint, _ := info["MountPoint"].(string)

		var diskType, iface string
		switch {
		case strings.Contains(bus, "NVMe") || strings.Contains(bus, "PCI"):
			diskType, iface = "nvme", "PCIe/NVMe"
		case strings.Contains(bus, "Apple Fabric") && solidState && internal:
			diskType, iface = "nvme", "Apple Fabric/NVMe"
		case strings.Contains(bus, "USB"):
			diskType, iface = "usb", "USB"
		case solidState:
			diskType, iface = "ssd", "SATA"
		default:
			diskType = "hdd"
			if bus != "" {
				iface = bus
			} else {
				iface = "Unknown"
			}
		}

		if mountPoint == "" {
			mountPoint = findDarwinMount(diskName)
		}

		disks = append(disks, DiskInfo{
			Device: device, Name: mediaName, DiskType: diskType,
			Interface: iface, SizeBytes: totalSize, MountPoint: mountPoint,
		})
	}

	// NFS mounts
	out, _ := runCmd("mount")
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(strings.ToLower(line), "(nfs") {
			parts := strings.SplitN(line, " on ", 2)
			if len(parts) < 2 {
				continue
			}
			remote := strings.TrimSpace(parts[0])
			rest := strings.SplitN(parts[1], " (", 2)
			mpoint := strings.TrimSpace(rest[0])
			disks = append(disks, DiskInfo{
				Device: remote, Name: remote, DiskType: "nfs",
				Interface: "Network", MountPoint: mpoint,
			})
		}
	}

	return disks
}

// findDarwinMount traces APFS containers to find mount points for a physical disk.
func findDarwinMount(diskName string) string {
	relatedDisks := map[string]bool{diskName: true}

	// Trace APFS containers
	raw, err := runCmdRaw("diskutil", "apfs", "list", "-plist")
	if err == nil {
		jsonData, err := plistToJSON(raw)
		if err == nil {
			var apfs map[string]interface{}
			if json.Unmarshal(jsonData, &apfs) == nil {
				containers, _ := apfs["Containers"].([]interface{})
				for _, c := range containers {
					cont, _ := c.(map[string]interface{})
					stores, _ := cont["PhysicalStores"].([]interface{})
					belongs := false
					for _, s := range stores {
						store, _ := s.(map[string]interface{})
						sid, _ := store["DeviceIdentifier"].(string)
						if strings.HasPrefix(sid, diskName) {
							belongs = true
							break
						}
					}
					if belongs {
						cref, _ := cont["ContainerReference"].(string)
						if cref != "" {
							relatedDisks[cref] = true
						}
					}
				}
			}
		}
	}

	// Check direct partitions
	raw, err = runCmdRaw("diskutil", "list", "-plist", diskName)
	if err == nil {
		jsonData, err := plistToJSON(raw)
		if err == nil {
			var dl map[string]interface{}
			if json.Unmarshal(jsonData, &dl) == nil {
				entries, _ := dl["AllDisksAndPartitions"].([]interface{})
				for _, e := range entries {
					entry, _ := e.(map[string]interface{})
					if mp, ok := entry["MountPoint"].(string); ok && mp != "" {
						return mp
					}
					parts, _ := entry["Partitions"].([]interface{})
					for _, p := range parts {
						part, _ := p.(map[string]interface{})
						if mp, ok := part["MountPoint"].(string); ok && mp != "" {
							return mp
						}
					}
				}
			}
		}
	}

	// Parse mount output for related disks
	out, err := runCmd("mount")
	if err != nil {
		return ""
	}
	best := ""
	for _, line := range strings.Split(out, "\n") {
		for rd := range relatedDisks {
			if strings.HasPrefix(line, fmt.Sprintf("/dev/%ss", rd)) ||
				strings.HasPrefix(line, fmt.Sprintf("/dev/%s ", rd)) {
				parts := strings.SplitN(line, " on ", 2)
				if len(parts) >= 2 {
					rest := strings.SplitN(parts[1], " (", 2)
					mpoint := strings.TrimSpace(rest[0])
					if mpoint == "/" {
						return "/"
					}
					if best == "" || len(mpoint) < len(best) {
						best = mpoint
					}
				}
			}
		}
	}
	return best
}

func matchDevicePlatform(device string, disks []DiskInfo) *DiskInfo {
	// macOS: trace synthesized APFS volumes back to physical disks
	// e.g., /dev/disk3s1 -> container disk3 -> physical store disk0s2 -> disk0
	raw, err := runCmdRaw("diskutil", "apfs", "list", "-plist")
	if err != nil {
		return nil
	}
	jsonData, err := plistToJSON(raw)
	if err != nil {
		return nil
	}
	var apfs map[string]interface{}
	if json.Unmarshal(jsonData, &apfs) != nil {
		return nil
	}

	devBase := strings.TrimPrefix(device, "/dev/")
	containers, _ := apfs["Containers"].([]interface{})
	for _, c := range containers {
		cont, _ := c.(map[string]interface{})
		cref, _ := cont["ContainerReference"].(string)
		if cref == "" || !strings.HasPrefix(devBase, cref) {
			continue
		}
		// This device belongs to this container. Find physical disk.
		stores, _ := cont["PhysicalStores"].([]interface{})
		for _, s := range stores {
			store, _ := s.(map[string]interface{})
			sid, _ := store["DeviceIdentifier"].(string)
			// Extract base disk name (disk0s2 -> disk0)
			base := extractBaseDisk(sid)
			physDev := "/dev/" + base
			for _, d := range disks {
				if d.Device == physDev {
					diskCopy := d
					diskCopy.Device = device
					diskCopy.MountPoint = ""
					return &diskCopy
				}
			}
		}
	}
	return nil
}

// extractBaseDisk gets "disk0" from "disk0s2"
func extractBaseDisk(s string) string {
	// Format: diskNsN or diskN
	if !strings.HasPrefix(s, "disk") {
		return s
	}
	for i := 4; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return s[:i]
		}
	}
	return s
}

func getPartitionSizePlatform(path string) int64 {
	var stat syscall.Statfs_t
	if syscall.Statfs(path, &stat) != nil {
		return 0
	}
	return int64(stat.Blocks) * int64(stat.Bsize)
}
