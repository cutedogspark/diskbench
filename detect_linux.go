//go:build linux

package main

// Uses lsblk JSON output for block device detection

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// lsblk JSON structures
type lsblkOutput struct {
	BlockDevices []lsblkDevice `json:"blockdevices"`
}

type lsblkDevice struct {
	Name       string        `json:"name"`
	Type       string        `json:"type"`
	Size       string        `json:"size"`
	Model      *string       `json:"model"`
	Serial     *string       `json:"serial"`
	Tran       *string       `json:"tran"`
	Rota       *bool         `json:"rota"`
	MountPoint *string       `json:"mountpoint"`
	Children   []lsblkDevice `json:"children"`
}

func detectDisksPlatform() []DiskInfo {
	var disks []DiskInfo

	// Block devices via lsblk
	out, err := runCmd("lsblk", "-J", "-o", "NAME,TYPE,SIZE,MODEL,SERIAL,TRAN,ROTA,MOUNTPOINT")
	if err == nil {
		var lsblk lsblkOutput
		if json.Unmarshal([]byte(out), &lsblk) == nil {
			for _, dev := range lsblk.BlockDevices {
				if dev.Type != "disk" {
					continue
				}

				tran := ""
				if dev.Tran != nil {
					tran = strings.ToLower(*dev.Tran)
				}
				rota := true
				if dev.Rota != nil {
					rota = *dev.Rota
				}

				var diskType, iface string
				switch {
				case tran == "nvme" || strings.HasPrefix(dev.Name, "nvme"):
					diskType, iface = "nvme", "PCIe/NVMe"
				case tran == "usb":
					diskType, iface = "usb", "USB"
				case tran == "sata" || tran == "ata":
					iface = "SATA"
					if rota {
						diskType = "hdd"
					} else {
						diskType = "ssd"
					}
				default:
					if rota {
						diskType = "hdd"
					} else {
						diskType = "ssd"
					}
					if tran != "" {
						iface = strings.ToUpper(tran)
					} else {
						// Try to detect RAID controller or SAS via sysfs
						iface = detectInterfaceSysfs(dev.Name)
					}
				}

				model := "Unknown"
				if dev.Model != nil {
					model = strings.TrimSpace(*dev.Model)
				}
				serial := ""
				if dev.Serial != nil {
					serial = *dev.Serial
				}

				mount := ""
				if dev.MountPoint != nil {
					mount = *dev.MountPoint
				}
				if mount == "" {
					for _, child := range dev.Children {
						if child.MountPoint != nil && *child.MountPoint != "" {
							mount = *child.MountPoint
							break
						}
					}
				}

				disks = append(disks, DiskInfo{
					Device:     "/dev/" + dev.Name,
					Name:       model,
					DiskType:   diskType,
					Interface:  iface,
					SizeBytes:  parseLsblkSize(dev.Size),
					MountPoint: mount,
					Serial:     serial,
				})
			}
		}
	}

	// NFS mounts
	out, err = runCmd("mount", "-t", "nfs,nfs4")
	if err == nil {
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, " on ", 2)
			if len(parts) < 2 {
				continue
			}
			remote := strings.TrimSpace(parts[0])
			rest := strings.SplitN(parts[1], " type ", 2)
			mpoint := strings.TrimSpace(rest[0])
			disks = append(disks, DiskInfo{
				Device: remote, Name: remote, DiskType: "nfs",
				Interface: "Network", MountPoint: mpoint,
			})
		}
	}

	return disks
}

func matchDevicePlatform(device string, disks []DiskInfo) *DiskInfo {
	// Linux: /dev/sda1 -> /dev/sda
	return nil // Generic matching in detect.go handles this
}

func getPartitionSizePlatform(path string) int64 {
	var stat syscall.Statfs_t
	if syscall.Statfs(path, &stat) != nil {
		return 0
	}
	return int64(stat.Blocks) * int64(stat.Bsize)
}

// detectInterfaceSysfs tries to determine the interface type from sysfs.
// Useful for RAID controllers and SAS devices where lsblk TRAN is empty.
func detectInterfaceSysfs(devName string) string {
	sysPath := filepath.Join("/sys/block", devName, "device")

	// Check vendor
	vendor := readSysfsFile(filepath.Join(sysPath, "vendor"))

	// Check model for RAID keywords
	model := readSysfsFile(filepath.Join(sysPath, "model"))

	combined := strings.ToUpper(vendor + " " + model)

	// Known RAID controller patterns
	raidKeywords := []string{"RAID", "PERC", "UCSC-RAID", "MEGARAID", "LOGICAL", "VIRTUAL", "AVAGO", "LSI"}
	for _, kw := range raidKeywords {
		if strings.Contains(combined, kw) {
			return "RAID"
		}
	}

	// Check if SAS transport
	sasPath := filepath.Join(sysPath, "sas_address")
	if _, err := os.Stat(sasPath); err == nil {
		return "SAS"
	}

	// Check driver symlink for hints
	driverLink, err := os.Readlink(filepath.Join(sysPath, "driver"))
	if err == nil {
		driver := filepath.Base(driverLink)
		driverUpper := strings.ToUpper(driver)
		if strings.Contains(driverUpper, "MEGARAID") || strings.Contains(driverUpper, "MPTRAID") ||
			strings.Contains(driverUpper, "AACRAID") || strings.Contains(driverUpper, "HPSA") {
			return "RAID"
		}
		if strings.Contains(driverUpper, "MPT3SAS") || strings.Contains(driverUpper, "SAS") {
			return "SAS"
		}
	}

	return "Unknown"
}

func readSysfsFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
