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
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shirou/gopsutil/v3/disk"
	psnet "github.com/shirou/gopsutil/v3/net"
	"github.com/stretchr/testify/require"
)

// writeFile writes content to path under root, creating its directories.
func writeFile(t *testing.T, root, path, content string) {
	t.Helper()
	full := filepath.Join(root, path)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

// diskstatsLine formats a /proc/diskstats line for name with the given sectors read and written.
func diskstatsLine(minor int, name string, sectorsRead, sectorsWritten int) string {
	return fmt.Sprintf("259 %d %s 10 0 %d 5 20 0 %d 7 0 12 12 0 0 0 0\n", minor, name, sectorsRead, sectorsWritten)
}

// Test_sumDiskCounter_countsWholeDisksOnly covers the partitions, loop and device-mapper devices that
// /proc/diskstats lists alongside each disk. Their I/O is also counted on the disk under them.
func Test_sumDiskCounter_countsWholeDisksOnly(t *testing.T) {
	sys := t.TempDir()
	for _, dev := range []string{"nvme0n1", "sda"} {
		require.NoError(t, os.MkdirAll(filepath.Join(sys, "block", dev, "device"), 0o755))
	}
	for _, dev := range []string{"loop0", "dm-0"} {
		require.NoError(t, os.MkdirAll(filepath.Join(sys, "block", dev), 0o755))
	}

	proc := t.TempDir()
	writeFile(t, proc, "diskstats",
		diskstatsLine(0, "nvme0n1", 100, 1000)+
			diskstatsLine(1, "nvme0n1p1", 60, 600)+
			diskstatsLine(2, "nvme0n1p2", 40, 400)+
			diskstatsLine(3, "sda", 7, 70)+
			diskstatsLine(4, "sda1", 7, 70)+
			diskstatsLine(5, "loop0", 3, 30)+
			diskstatsLine(6, "dm-0", 40, 400))

	t.Setenv("HOST_SYS", sys)
	t.Setenv("HOST_PROC", proc)

	require.Equal(t, int64((100+7)*512), sumDiskCounter(func(s disk.IOCountersStat) uint64 { return s.ReadBytes }))
	require.Equal(t, int64((1000+70)*512), sumDiskCounter(func(s disk.IOCountersStat) uint64 { return s.WriteBytes }))
}

// Test_wholeDisks_areListedInDiskstats runs against this host's sysfs and /proc/diskstats. A whole disk that sysfs
// names differently would match no counter, and the disk would read as idle.
func Test_wholeDisks_areListedInDiskstats(t *testing.T) {
	disks := wholeDisks()
	if len(disks) == 0 {
		t.Skip("no whole disks visible on this host")
	}
	counters, err := disk.IOCounters()
	require.NoError(t, err)
	for _, name := range disks {
		require.Contains(t, counters, name)
	}
}

func Test_aggregateNetCounter_leavesOutLoopback(t *testing.T) {
	var loopback string
	ifaces, err := net.Interfaces()
	require.NoError(t, err)
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			loopback = iface.Name
		}
	}
	if loopback == "" {
		t.Skip("no loopback interface in this network namespace")
	}

	proc := t.TempDir()
	writeFile(t, proc, "net/dev", strings.Join([]string{
		"Inter-|   Receive                                                |  Transmit",
		" face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed",
		fmt.Sprintf("%6s: 900000 9000 0 1 0 0 0 0 900000 9000 0 1 0 0 0 0", loopback),
		"  eth9: 5000 50 0 3 0 0 0 0 7000 70 0 4 0 0 0 0",
		"  eth8: 100 1 0 0 0 0 0 0 200 2 0 0 0 0 0 0",
	}, "\n")+"\n")
	t.Setenv("HOST_PROC", proc)

	require.Equal(t, int64(5100), aggregateNetCounter(func(s psnet.IOCountersStat) uint64 { return s.BytesRecv }))
	require.Equal(t, int64(7200), aggregateNetCounter(func(s psnet.IOCountersStat) uint64 { return s.BytesSent }))
	require.Equal(t, int64(3), aggregateNetCounter(func(s psnet.IOCountersStat) uint64 { return s.Dropin }))
	require.Equal(t, int64(4), aggregateNetCounter(func(s psnet.IOCountersStat) uint64 { return s.Dropout }))
}
