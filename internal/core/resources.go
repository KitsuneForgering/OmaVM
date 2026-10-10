package core

import (
	"os"
	"runtime"
	"strconv"
	"strings"
)

// HostMemoryMiB reads physical RAM, rather than currently free RAM: VM
// defaults should not jump around as other applications open and close.
func HostMemoryMiB() (int, bool) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(line, "MemTotal:"); ok {
			fields := strings.Fields(rest)
			if len(fields) != 2 || fields[1] != "kB" {
				return 0, false
			}
			kib, err := strconv.Atoi(fields[0])
			return kib / 1024, err == nil && kib > 0
		}
	}
	return 0, false
}

func halfMachineResources(hostCPUs, hostMemoryMiB int) (int, int) {
	cpus := hostCPUs / 2
	if cpus < 1 {
		cpus = 1
	}
	if cpus > 64 {
		cpus = 64
	}
	memory := hostMemoryMiB / 2
	if memory < 256 {
		memory = 256
	}
	return cpus, memory
}

// DefaultMachineResources is the allocation for an unconfigured Machine.
// A missing memory reading retains the previous 2 GiB fallback.
func DefaultMachineResources() (int, int) {
	memory, ok := HostMemoryMiB()
	if !ok {
		memory = 4096
	}
	return halfMachineResources(runtime.NumCPU(), memory)
}
