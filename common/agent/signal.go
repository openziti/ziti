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

// Package agent contains signals used to communicate to the gops agents.
package agent

const (
	// StackTrace represents a command to print stack trace.
	StackTrace = byte(0x1)

	// GC runs the garbage collector.
	GC = byte(0x2)

	// MemStats reports memory stats.
	MemStats = byte(0x3)

	// Version prints the Go version.
	Version = byte(0x4)

	// HeapProfile starts `go tool pprof` with the current memory profile.
	HeapProfile = byte(0x5)

	// CPUProfile starts `go tool pprof` with the current CPU profile
	CPUProfile = byte(0x6)

	// Stats returns Go runtime statistics such as number of goroutines, GOMAXPROCS, and NumCPU.
	Stats = byte(0x7)

	// Trace starts the Go execution tracer, waits 5 seconds and launches the trace tool.
	Trace = byte(0x8)

	// BinaryDump returns running binary file.
	BinaryDump = byte(0x9)

	// AppInfo returns application information
	AppInfo = byte(0xa)

	// HeapDump dumps the full heap
	HeapDump = byte(0xb)

	// fill in b-f

	// SetGCPercent sets the garbage collection target percentage.
	SetGCPercent = byte(0x10)

	// SetLogLevel sets the logrus level
	SetLogLevel = byte(0x11)

	// CustomOp reserved for application-specific operations
	CustomOp = byte(0x12)

	// SetChannelLogLevel sets the log level for a channel
	SetChannelLogLevel = byte(0x13)

	// ClearChannelLogLevel clears the log level for a channel
	ClearChannelLogLevel = byte(0x14)

	// CustomOpAsync reserved for application specific operations which execute async
	CustomOpAsync = byte(0x15)
)
