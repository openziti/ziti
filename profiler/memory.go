/*
	Copyright NetFoundry, Inc.

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

package profiler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/util/info"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"time"
)

type Monitor struct {
	Alloc,
	TotalAlloc,
	Sys,
	Mallocs,
	Frees,
	LiveObjects,
	PauseTotalNs uint64
	NumGC        uint32
	NumGoroutine int
}

type Memory struct {
	path      string
	interval  time.Duration
	ctr       int
	shutdownC <-chan struct{}
}

// NewGoroutineMonitor can be used to track down goroutine leaks
//
func NewGoroutineMonitor(interval time.Duration) {
	var m Monitor
	var rtm runtime.MemStats
	for {
		<-time.After(interval)

		// Read full mem stats
		runtime.ReadMemStats(&rtm)

		// Number of goroutines
		m.NumGoroutine = runtime.NumGoroutine()

		// Misc memory stats
		m.Alloc = rtm.Alloc
		m.TotalAlloc = rtm.TotalAlloc
		m.Sys = rtm.Sys
		m.Mallocs = rtm.Mallocs
		m.Frees = rtm.Frees

		// Live objects = Mallocs - Frees
		m.LiveObjects = m.Mallocs - m.Frees

		// GC Stats
		m.PauseTotalNs = rtm.PauseTotalNs
		m.NumGC = rtm.NumGC

		fmt.Println("----------------------------------------------------------")

		// Just encode to json and print
		b, _ := json.Marshal(m)
		fmt.Println(string(b))

		stack := debug.Stack()
		fmt.Println(string(stack))
		buf := new(bytes.Buffer)
		pprof.Lookup("goroutine").WriteTo(buf, 2)
		fmt.Println(buf.String())
	}
}

func NewMemory(path string, interval time.Duration) *Memory {
	return NewMemoryWithShutdown(path, interval, nil)
}

func NewMemoryWithShutdown(path string, interval time.Duration, shutdownC <-chan struct{}) *Memory {
	// go NewGoroutineMonitor(interval) // disable for now
	return &Memory{path: path, interval: interval, ctr: 0, shutdownC: shutdownC}
}

func (memory *Memory) Run() {
	log := pfxlog.Logger()
	log.Infof("memory profiling to [%s]", memory.path)
	tick := time.NewTicker(memory.interval)
	defer tick.Stop()
	for {
		select {
		case _, ok := <-memory.shutdownC:
			if !ok {
				return
			}
		case <-tick.C:
			memory.stats()
			if err := memory.captureProfile(); err != nil {
				log.Errorf("error capturing memory profile (%s)", err)
			}
		}
	}
}

func (memory *Memory) stats() {
	memStats := &runtime.MemStats{}
	runtime.ReadMemStats(memStats)
	pfxlog.Logger().Infof("runtime.HeapSys=[%s], runtime.HeapAlloc=[%s], runtime.HeapIdle=[%s]",
		info.ByteCount(int64(memStats.HeapSys)),
		info.ByteCount(int64(memStats.HeapAlloc)),
		info.ByteCount(int64(memStats.HeapIdle)))
}

func (memory *Memory) captureProfile() error {
	var ndx string
	if (memory.ctr % 2) == 0 {
		ndx = "-0"
	} else {
		ndx = "-1"
	}
	runtime.GC()
	f, err := os.Create(memory.path + ndx)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := pprof.WriteHeapProfile(f); err != nil {
		return err
	}
	memory.ctr++
	return nil
}
