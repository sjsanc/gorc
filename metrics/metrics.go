package metrics

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Metrics struct {
	CPULoad     CPUMetrics    `json:"cpu"`
	Memory      MemoryMetrics `json:"memory"`
	Disk        DiskMetrics   `json:"disk"`
	CollectedAt time.Time     `json:"collected_at"`
}

type CPUMetrics struct {
	Load1  float64 `json:"load_1m"`
	Load5  float64 `json:"load_5m"`
	Load15 float64 `json:"load_15m"`
}

type MemoryMetrics struct {
	TotalBytes     uint64  `json:"total_bytes"`
	AvailableBytes uint64  `json:"available_bytes"`
	UsedBytes      uint64  `json:"used_bytes"`
	UsedPercent    float64 `json:"used_percent"`
}

type DiskMetrics struct {
	TotalBytes     uint64  `json:"total_bytes"`
	UsedBytes      uint64  `json:"used_bytes"`
	AvailableBytes uint64  `json:"available_bytes"`
	UsedPercent    float64 `json:"used_percent"`
	Path           string  `json:"path"`
}

type Collector interface {
	Collect() (*Metrics, error)
}

type SystemCollector struct{}

func NewSystemCollector() (*SystemCollector, error) {
	return &SystemCollector{}, nil
}

func (c *SystemCollector) Collect() (*Metrics, error) {
	m := &Metrics{
		CollectedAt: time.Now(),
	}

	var collectionErrors []string

	cpu, err := collectCPU()
	if err != nil {
		collectionErrors = append(collectionErrors, fmt.Sprintf("cpu: %v", err))
	} else {
		m.CPULoad = cpu
	}

	mem, err := collectMemory()
	if err != nil {
		collectionErrors = append(collectionErrors, fmt.Sprintf("memory: %v", err))
	} else {
		m.Memory = mem
	}

	disk, err := collectDisk("/")
	if err != nil {
		collectionErrors = append(collectionErrors, fmt.Sprintf("disk: %v", err))
	} else {
		m.Disk = disk
	}

	if len(collectionErrors) > 0 {
		return m, fmt.Errorf("collection errors: %s", strings.Join(collectionErrors, "; "))
	}

	return m, nil
}

func collectCPU() (CPUMetrics, error) {
	file, err := os.Open("/proc/loadavg")
	if err != nil {
		return CPUMetrics{}, fmt.Errorf("failed to open /proc/loadavg: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return CPUMetrics{}, fmt.Errorf("failed to read /proc/loadavg")
	}

	line := scanner.Text()
	fields := strings.Fields(line)
	if len(fields) < 3 {
		return CPUMetrics{}, fmt.Errorf("unexpected format in /proc/loadavg")
	}

	load1, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return CPUMetrics{}, fmt.Errorf("failed to parse 1min load: %v", err)
	}

	load5, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return CPUMetrics{}, fmt.Errorf("failed to parse 5min load: %v", err)
	}

	load15, err := strconv.ParseFloat(fields[2], 64)
	if err != nil {
		return CPUMetrics{}, fmt.Errorf("failed to parse 15min load: %v", err)
	}

	return CPUMetrics{
		Load1:  load1,
		Load5:  load5,
		Load15: load15,
	}, nil
}

func collectMemory() (MemoryMetrics, error) {
	var info syscall.Sysinfo_t
	err := syscall.Sysinfo(&info)
	if err != nil {
		return MemoryMetrics{}, fmt.Errorf("syscall.Sysinfo failed: %v", err)
	}

	totalBytes := info.Totalram * uint64(info.Unit)
	freeBytes := info.Freeram * uint64(info.Unit)
	bufferBytes := info.Bufferram * uint64(info.Unit)

	availableBytes := freeBytes + bufferBytes
	usedBytes := totalBytes - availableBytes
	usedPercent := 0.0
	if totalBytes > 0 {
		usedPercent = (float64(usedBytes) / float64(totalBytes)) * 100.0
	}

	return MemoryMetrics{
		TotalBytes:     totalBytes,
		AvailableBytes: availableBytes,
		UsedBytes:      usedBytes,
		UsedPercent:    usedPercent,
	}, nil
}

func collectDisk(path string) (DiskMetrics, error) {
	var stat syscall.Statfs_t
	err := syscall.Statfs(path, &stat)
	if err != nil {
		return DiskMetrics{}, fmt.Errorf("syscall.Statfs failed for %s: %v", path, err)
	}

	totalBytes := stat.Blocks * uint64(stat.Bsize)
	availableBytes := stat.Bavail * uint64(stat.Bsize)
	usedBytes := totalBytes - (stat.Bfree * uint64(stat.Bsize))
	usedPercent := 0.0
	if totalBytes > 0 {
		usedPercent = (float64(usedBytes) / float64(totalBytes)) * 100.0
	}

	return DiskMetrics{
		TotalBytes:     totalBytes,
		UsedBytes:      usedBytes,
		AvailableBytes: availableBytes,
		UsedPercent:    usedPercent,
		Path:           path,
	}, nil
}
