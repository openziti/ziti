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

package profiler

import (
	"github.com/michaelquigley/pfxlog"
	"os"
	"os/signal"
	"runtime/pprof"
	"syscall"
)

type CPU struct {
	path      string
	shutdownC chan struct{}
}

func NewCPU(path string) (*CPU, error) {
	return NewCPUWithShutdown(path, nil)
}

func NewCPUWithShutdown(path string, shutdownC chan struct{}) (*CPU, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		return nil, err
	}
	pfxlog.Logger().Infof("cpu profiling to [%s]", path)
	return &CPU{path: path, shutdownC: shutdownC}, nil
}

func (cpu *CPU) Run() {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGUSR2)

	select {
	case <-signalChan:
	case <-cpu.shutdownC:
	}

	pprof.StopCPUProfile()
	pfxlog.Logger().Info("stopped profiling cpu")
}
