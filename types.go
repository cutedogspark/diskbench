package main

// DiskInfo represents a detected disk or storage device.
type DiskInfo struct {
	Device     string
	Name       string
	DiskType   string // nvme, ssd, hdd, usb, nfs
	Interface  string // PCIe/NVMe, SATA, USB, Network, Apple Fabric/NVMe
	SizeBytes  int64
	MountPoint string
	Serial     string
}

// HealthAttr is a single SMART attribute for display.
type HealthAttr struct {
	Name   string
	Value  string
	Status string // OK, WARN, FAIL
}

// HealthResult holds the outcome of a SMART health check.
type HealthResult struct {
	Status             string // HEALTHY, WARNING, CRITICAL, UNKNOWN, N/A
	Temperature        int
	PowerOnHours       int
	WearLevel          int // percentage remaining (100 = new)
	ReallocatedSectors int
	MediaErrors        int
	Attributes         []HealthAttr
	Message            string
}

// SpeedResult holds sequential read/write benchmark results.
type SpeedResult struct {
	ReadMBPS  float64
	WriteMBPS float64
	TestSize  int64
	BlockSize int
	DirectIO  bool
}

// IOPSResult holds random I/O benchmark results.
type IOPSResult struct {
	Label          string // QD1, QD4
	ReadIOPS       float64
	WriteIOPS      float64
	ReadLatencyUS  float64 // microseconds
	WriteLatencyUS float64
	QueueDepth     int
	BlockSize      int
	Duration       float64
}
