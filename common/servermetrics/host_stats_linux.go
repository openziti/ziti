//go:build linux

/*
	Copyright NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package servermetrics

import (
	"net"
	"os"
	"path/filepath"
	"runtime"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/metrics"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	psnet "github.com/shirou/gopsutil/v3/net"
)

// RegisterHostStats wires host- and process-level metrics into the registry
// as poll-time FuncGauges. The values are read on the registry's normal
// reporting cadence; no background goroutine is started. Safe with a nil
// registry or a disabled config.
//
// Registered metrics:
//
//	process.goroutines           - live goroutine count
//	host.cpu.percent             - aggregate CPU utilization since last sample
//	host.load.1m / 5m / 15m      - load averages
//	host.mem.total / used        - bytes
//	host.mem.available           - bytes the kernel can hand out without swapping
//	host.mem.used_percent        - used / total as a percentage
//	host.disk.read_bytes         - cumulative bytes read across whole disks
//	host.disk.write_bytes        - cumulative bytes written across whole disks
//	host.disk.available_bytes    - free space on the root filesystem
//	host.disk.used_percent       - root filesystem usage as a percentage
//	host.net.rx_bytes / tx_bytes - cumulative bytes across all interfaces but loopback
//	host.net.rx_drops / tx_drops - cumulative packet drops across all interfaces but loopback
//
// Disk and network counters are cumulative; consumers compute rates from
// successive samples.
func RegisterHostStats(reg metrics.Registry, cfg HostStatsConfig) {
	if !cfg.Enabled || reg == nil {
		return
	}

	reg.FuncGauge("process.goroutines", func() int64 {
		return int64(runtime.NumGoroutine())
	})

	reg.FuncGaugeFloat64("host.cpu.percent", func() float64 {
		// Interval=0 returns the delta since the previous call. First call
		// after process start returns 0, which is fine.
		vals, err := cpu.Percent(0, false)
		if err != nil || len(vals) == 0 {
			return 0
		}
		return vals[0]
	})

	reg.FuncGaugeFloat64("host.load.1m", func() float64 {
		if avg, err := load.Avg(); err == nil {
			return avg.Load1
		}
		return 0
	})
	reg.FuncGaugeFloat64("host.load.5m", func() float64 {
		if avg, err := load.Avg(); err == nil {
			return avg.Load5
		}
		return 0
	})
	reg.FuncGaugeFloat64("host.load.15m", func() float64 {
		if avg, err := load.Avg(); err == nil {
			return avg.Load15
		}
		return 0
	})

	reg.FuncGauge("host.mem.total", func() int64 {
		if m, err := mem.VirtualMemory(); err == nil {
			return int64(m.Total)
		}
		return 0
	})
	reg.FuncGauge("host.mem.used", func() int64 {
		if m, err := mem.VirtualMemory(); err == nil {
			return int64(m.Used)
		}
		return 0
	})
	reg.FuncGauge("host.mem.available", func() int64 {
		if m, err := mem.VirtualMemory(); err == nil {
			return int64(m.Available)
		}
		return 0
	})
	reg.FuncGaugeFloat64("host.mem.used_percent", func() float64 {
		if m, err := mem.VirtualMemory(); err == nil {
			return m.UsedPercent
		}
		return 0
	})

	reg.FuncGauge("host.disk.read_bytes", func() int64 {
		return sumDiskCounter(func(s disk.IOCountersStat) uint64 { return s.ReadBytes })
	})
	reg.FuncGauge("host.disk.write_bytes", func() int64 {
		return sumDiskCounter(func(s disk.IOCountersStat) uint64 { return s.WriteBytes })
	})

	// Filesystem usage on the root mount. Catches log-volume runaway,
	// bolt-db growth, etc. before the disk fills and the controller crashes.
	reg.FuncGauge("host.disk.available_bytes", func() int64 {
		if u, err := disk.Usage("/"); err == nil {
			return int64(u.Free)
		}
		return 0
	})
	reg.FuncGaugeFloat64("host.disk.used_percent", func() float64 {
		if u, err := disk.Usage("/"); err == nil {
			return u.UsedPercent
		}
		return 0
	})

	reg.FuncGauge("host.net.rx_bytes", func() int64 {
		return aggregateNetCounter(func(s psnet.IOCountersStat) uint64 { return s.BytesRecv })
	})
	reg.FuncGauge("host.net.tx_bytes", func() int64 {
		return aggregateNetCounter(func(s psnet.IOCountersStat) uint64 { return s.BytesSent })
	})
	reg.FuncGauge("host.net.rx_drops", func() int64 {
		return aggregateNetCounter(func(s psnet.IOCountersStat) uint64 { return s.Dropin })
	})
	reg.FuncGauge("host.net.tx_drops", func() int64 {
		return aggregateNetCounter(func(s psnet.IOCountersStat) uint64 { return s.Dropout })
	})

	pfxlog.Logger().Info("host stats metrics enabled")
}

// sumDiskCounter sums a uint64 field across the host's whole disks. Cardinality
// per-device is intentionally hidden to keep the metric namespace bounded.
func sumDiskCounter(extract func(disk.IOCountersStat) uint64) int64 {
	disks := wholeDisks()
	if len(disks) == 0 {
		return 0
	}
	counters, err := disk.IOCounters(disks...)
	if err != nil {
		return 0
	}
	var sum uint64
	for _, c := range counters {
		sum += extract(c)
	}
	return int64(sum)
}

// wholeDisks returns the names of the block devices backed by a device, read from
// the sysfs under HOST_SYS, as gopsutil reads /proc under HOST_PROC. /proc/diskstats
// also lists partitions and virtual devices such as loop, device-mapper, md and
// zram, whose I/O is either counted again on the disk beneath them or is not disk
// I/O at all. Partitions are not in /sys/block, and virtual devices have no device.
// Empty when sysfs cannot be read.
func wholeDisks() []string {
	root := os.Getenv("HOST_SYS")
	if root == "" {
		root = "/sys"
	}
	block := filepath.Join(root, "block")

	entries, err := os.ReadDir(block)
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		if _, err := os.Stat(filepath.Join(block, entry.Name(), "device")); err == nil {
			names = append(names, entry.Name())
		}
	}
	return names
}

// aggregateNetCounter returns a uint64 field summed across every interface but
// loopback, whose traffic never leaves the host. Traffic forwarded between
// interfaces, as to containers through a bridge, is counted on each of them.
func aggregateNetCounter(extract func(psnet.IOCountersStat) uint64) int64 {
	counters, err := psnet.IOCounters(true)
	if err != nil {
		return 0
	}
	loopbacks := loopbackInterfaces()
	var sum uint64
	for _, c := range counters {
		if !loopbacks[c.Name] {
			sum += extract(c)
		}
	}
	return int64(sum)
}

// loopbackInterfaces returns the names of this network namespace's loopback
// interfaces. Empty when the interfaces cannot be listed.
func loopbackInterfaces() map[string]bool {
	result := map[string]bool{}
	ifaces, err := net.Interfaces()
	if err != nil {
		return result
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			result[iface.Name] = true
		}
	}
	return result
}
