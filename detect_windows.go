//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

func detectDisksPlatform() []DiskInfo {
	var disks []DiskInfo

	// Try PowerShell first
	out, err := runCmd("powershell", "-NoProfile", "-Command",
		"Get-PhysicalDisk | Select-Object DeviceId,FriendlyName,MediaType,BusType,Size,HealthStatus | ConvertTo-Json")

	if err == nil && strings.TrimSpace(out) != "" {
		// Could be single object or array
		var raw json.RawMessage
		if json.Unmarshal([]byte(out), &raw) == nil {
			var psDiskList []map[string]interface{}
			if json.Unmarshal(raw, &psDiskList) != nil {
				// Try single object
				var single map[string]interface{}
				if json.Unmarshal(raw, &single) == nil {
					psDiskList = []map[string]interface{}{single}
				}
			}

			for _, pd := range psDiskList {
				devID := ""
				if v, ok := pd["DeviceId"].(float64); ok {
					devID = fmt.Sprintf("%d", int(v))
				} else if v, ok := pd["DeviceId"].(string); ok {
					devID = v
				}

				name, _ := pd["FriendlyName"].(string)
				if name == "" {
					name = "Unknown"
				}

				busType, _ := pd["BusType"].(string)
				// BusType could be numeric
				if bt, ok := pd["BusType"].(float64); ok {
					switch int(bt) {
					case 17:
						busType = "NVMe"
					case 11:
						busType = "SATA"
					case 7:
						busType = "USB"
					}
				}

				mediaType, _ := pd["MediaType"].(string)
				if mt, ok := pd["MediaType"].(float64); ok {
					switch int(mt) {
					case 4:
						mediaType = "SSD"
					case 3:
						mediaType = "HDD"
					}
				}

				sizeBytes := int64(0)
				if v, ok := pd["Size"].(float64); ok {
					sizeBytes = int64(v)
				}

				var diskType, iface string
				switch {
				case strings.Contains(busType, "NVMe"):
					diskType, iface = "nvme", "PCIe/NVMe"
				case strings.Contains(busType, "USB"):
					diskType, iface = "usb", "USB"
				case strings.Contains(mediaType, "SSD") || mediaType == "4":
					diskType, iface = "ssd", "SATA"
				default:
					diskType, iface = "hdd", busType
				}

				mount := findWindowsMount(devID)

				disks = append(disks, DiskInfo{
					Device:     `\\.\PhysicalDrive` + devID,
					Name:       name,
					DiskType:   diskType,
					Interface:  iface,
					SizeBytes:  sizeBytes,
					MountPoint: mount,
				})
			}
		}
	}

	// Fallback: wmic
	if len(disks) == 0 {
		disks = detectDisksWmic()
	}

	// Network shares
	out, err = runCmd("net", "use")
	if err == nil {
		for _, line := range strings.Split(out, "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 3 && len(fields[1]) == 2 && fields[1][1] == ':' {
				remote := fields[2]
				if strings.HasPrefix(remote, `\\`) {
					disks = append(disks, DiskInfo{
						Device: remote, Name: remote, DiskType: "nfs",
						Interface: "Network", MountPoint: fields[1] + `\`,
					})
				}
			}
		}
	}

	return disks
}

func detectDisksWmic() []DiskInfo {
	var disks []DiskInfo
	out, err := runCmd("wmic", "diskdrive", "get",
		"Caption,InterfaceType,MediaType,Size,Status,SerialNumber,Index", "/format:list")
	if err != nil {
		return disks
	}

	// Parse key=value blocks separated by blank lines
	current := make(map[string]string)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			if len(current) > 0 {
				diskFromWmic(current, &disks)
				current = make(map[string]string)
			}
			continue
		}
		if idx := strings.Index(line, "="); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			current[key] = val
		}
	}
	if len(current) > 0 {
		diskFromWmic(current, &disks)
	}

	return disks
}

func diskFromWmic(m map[string]string, disks *[]DiskInfo) {
	caption := m["Caption"]
	ifType := m["InterfaceType"]
	mediaType := m["MediaType"]
	sizeStr := m["Size"]
	index := m["Index"]
	serial := m["SerialNumber"]

	if caption == "" && index == "" {
		return
	}

	var diskType, iface string
	switch {
	case strings.Contains(ifType, "NVMe"):
		diskType, iface = "nvme", "PCIe/NVMe"
	case strings.Contains(ifType, "USB"):
		diskType, iface = "usb", "USB"
	case strings.Contains(mediaType, "SSD"):
		diskType, iface = "ssd", "SATA"
	default:
		diskType, iface = "hdd", ifType
	}

	sizeBytes := parseSize(sizeStr)
	mount := findWindowsMount(index)

	*disks = append(*disks, DiskInfo{
		Device:     `\\.\PhysicalDrive` + index,
		Name:       caption,
		DiskType:   diskType,
		Interface:  iface,
		SizeBytes:  sizeBytes,
		MountPoint: mount,
		Serial:     serial,
	})
}

func findWindowsMount(diskNum string) string {
	out, err := runCmd("powershell", "-NoProfile", "-Command",
		fmt.Sprintf(`Get-Partition -DiskNumber %s -ErrorAction SilentlyContinue | Get-Volume -ErrorAction SilentlyContinue | Select-Object -ExpandProperty DriveLetter`, diskNum))
	if err != nil {
		return ""
	}
	letter := strings.TrimSpace(out)
	if letter != "" && len(letter) <= 2 {
		return letter + `:\`
	}
	return ""
}

func matchDevicePlatform(device string, disks []DiskInfo) *DiskInfo {
	return nil // Windows devices are direct PhysicalDrive paths
}

func getPartitionSizePlatform(path string) int64 {
	pathp, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0
	}
	var free, total, available uint64
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GetDiskFreeSpaceExW")
	r, _, _ := proc.Call(
		uintptr(unsafe.Pointer(pathp)),
		uintptr(unsafe.Pointer(&available)),
		uintptr(unsafe.Pointer(&total)),
		uintptr(unsafe.Pointer(&free)),
	)
	if r == 0 {
		return 0
	}
	return int64(total)
}
