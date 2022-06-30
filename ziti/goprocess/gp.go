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

package goprocess

import (
	// "fmt"
	"os"
	"strings"
	"sync"

	goversion "rsc.io/goversion/version"

	"github.com/openziti/ziti/ziti/util"
	ps "github.com/keybase/go-ps"
)

// P represents a Go process.
type P struct {
	PID          int
	PPID         int
	Exec         string
	Path         string
	BuildVersion string
	Agent        bool
}

// FindAll returns all the Ziti processes currently running on this host.
func FindAll() []P {
	pss, err := ps.Processes()
	// fmt.Println("FindAll, err is: %s", err)

	if err != nil {
		return nil
	}

	var wg sync.WaitGroup
	wg.Add(len(pss))
	found := make(chan P)

	for _, pr := range pss {
		pr := pr
		go func() {
			defer wg.Done()

			path, version, agent, ok, err := isGo(pr)
			if err != nil {
				// TODO(jbd): Return a list of errors.
			}
			if !ok {
				return
			}
			if isZiti(pr) {
				found <- P{
					PID:          pr.Pid(),
					PPID:         pr.PPid(),
					Exec:         pr.Executable(),
					Path:         path,
					BuildVersion: version,
					Agent:        agent,
				}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(found)
	}()
	var results []P
	for p := range found {
		results = append(results, p)
	}
	return results
}

// Find finds info about the process identified with the given PID.
func Find(pid int) (p P, ok bool, err error) {
	pr, err := ps.FindProcess(pid)
	if err != nil {
		return P{}, false, err
	}
	path, version, agent, ok, err := isGo(pr)
	if !ok {
		return P{}, false, nil
	}
	return P{
		PID:          pr.Pid(),
		PPID:         pr.PPid(),
		Exec:         pr.Executable(),
		Path:         path,
		BuildVersion: version,
		Agent:        agent,
	}, true, nil
}

// isGo looks up the runtime.buildVersion symbol
// in the process' binary and determines if the process
// if a Go process or not. If the process is a Go process,
// it reports PID, binary name and full path of the binary.
func isGo(pr ps.Process) (path, version string, agent, ok bool, err error) {
	if pr.Pid() == 0 {
		// ignore system process
		return
	}
	path, err = pr.Path()
	if err != nil {
		return
	}
	var versionInfo goversion.Version
	versionInfo, err = goversion.ReadExe(path)
	if err != nil {
		return
	}
	ok = true
	version = versionInfo.Release
	pidfile, err := util.PIDFile(pr.Pid())
	if err == nil {
		_, err := os.Stat(pidfile)
		agent = err == nil
	}
	return path, version, agent, ok, nil
}

func isZiti(pr ps.Process) (ok bool) {
	// fmt.Println("isZiti, pr.Executable(): %s", pr.Executable())

	return strings.HasPrefix(pr.Executable(), "ziti-")
}
