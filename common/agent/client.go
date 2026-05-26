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

package agent

import (
	"encoding/json"
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/mitchellh/go-ps"
	"github.com/pkg/errors"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Process struct {
	Pid         int
	Executable  string
	UnixSocket  string
	AppId       string
	AppType     string
	AppAlias    string
	AppVersion  string
	Contactable bool
}

func (p *Process) String() string {
	return fmt.Sprintf("%v (pid %v)", p.Executable, p.Pid)
}

func GetGopsProcesses() ([]*Process, error) {
	var result []*Process
	matches, err := filepath.Glob(path.Join(os.TempDir(), fmt.Sprintf("%v.*.sock", SockPrefix)))
	if err != nil {
		return nil, err
	}
	re, err := regexp.Compile(`.*\.(?P<PID>\d+)\.sock`)
	if err != nil {
		return nil, err
	}
	for _, match := range matches {
		res := re.FindSubmatch([]byte(match))
		if len(res) != 0 {
			pid, err := strconv.Atoi(string(res[1]))
			if err != nil {
				return nil, err
			}
			// process is alive and is reachable by us
			if process, err := os.FindProcess(pid); err == nil {
				if isReachable(process) {
					proc, err := ps.FindProcess(pid)
					if err != nil {
						return nil, err
					}
					if proc == nil {
						// indicates an older executable no longer running...
						continue
					}
					gopsProcess := &Process{
						Pid:        pid,
						Executable: proc.Executable(),
						UnixSocket: match,
					}
					err = FillProcessInfo(gopsProcess)
					if err != nil {
						pfxlog.Logger().WithError(err).WithField("pid", pid).Info("unable to get app info")
					}
					result = append(result, gopsProcess)
				}
			}
		}
	}
	return result, nil
}

func GetUnixSockForPid(pid int) string {
	return path.Join(os.TempDir(), fmt.Sprintf("%v.%v.sock", SockPrefix, pid))
}

func tooManyProcsError(procs []*Process) error {
	var list []string
	for _, v := range procs {
		list = append(list, v.String())
	}
	builder := &strings.Builder{}
	builder.WriteString("too many gops-agent process found, specify one, found [")
	builder.WriteString(strings.Join(list, ", "))
	builder.WriteString("]")
	return errors.New(builder.String())
}

// ParseGopsAddress tries to parse the target string, be it remote host:port
// or local process's PID or executable name
func ParseGopsAddress(args []string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	target := args[0]
	if strings.Contains(target, ":") {
		return target, nil
	}

	// try to find port by pid then, connect to local
	num, err := strconv.Atoi(target)
	if err == nil {
		filePath := GetUnixSockForPid(num)
		if _, err := os.Stat(filePath); err == nil {
			return "unix:" + filePath, nil
		}
		return "tcp:127.0.0.1:" + target, nil
	}

	return target, nil
}

func FillProcessInfo(p *Process) error {
	conn, err := net.DialTimeout("unix", p.UnixSocket, 250*time.Millisecond)
	if err != nil {
		return err
	}
	p.Contactable = true

	return MakeRequestToConn(conn, AppInfo, nil, func(conn net.Conn) error {
		buf, err := io.ReadAll(conn)
		if err != nil {
			return err
		}
		if len(buf) == 0 {
			// not json, exit out
			return nil
		}
		m := map[string]string{}
		if err = json.Unmarshal(buf, &m); err != nil {
			return err
		}

		p.AppId = m["id"]
		p.AppType = m["type"]
		p.AppAlias = m["alias"]
		p.AppVersion = m["version"]

		return nil
	})
}

func MakeProcessRequest(p *Process, signal byte, params []byte, f func(conn net.Conn) error) error {
	conn, err := net.DialTimeout("unix", p.UnixSocket, 250*time.Millisecond)
	if err != nil {
		return err
	}
	return MakeRequestToConn(conn, signal, params, f)
}

func MakeRequest(addr string, signal byte, params []byte, out io.Writer) error {
	return MakeRequestF(addr, signal, params, func(conn net.Conn) error {
		_, err := io.Copy(out, conn)
		return err
	})
}

func MakeRequestF(addr string, signal byte, params []byte, f func(conn net.Conn) error) error {
	network := "tcp"
	if addr == "" {
		network = "unix"
		procs, err := GetGopsProcesses()
		if err != nil {
			return err
		}
		if len(procs) == 0 {
			return errors.New("no gops-agent processes found")
		}
		if len(procs) > 1 {
			return tooManyProcsError(procs)
		}
		addr = procs[0].UnixSocket
	} else {
		procs, err := GetGopsProcesses()
		found := false
		if err == nil {
			var filtered []*Process
			for _, proc := range procs {
				if strings.Contains(proc.Executable, addr) {
					filtered = append(filtered, proc)
				}
			}
			if len(filtered) > 1 {
				return tooManyProcsError(filtered)
			}
			if len(filtered) == 1 {
				found = true
				network = "unix"
				addr = filtered[0].UnixSocket
			}
		}

		if !found {
			parts := strings.SplitN(addr, ":", 2)
			if len(parts) == 2 {
				network = parts[0]
				addr = parts[1]
			}
		}
	}

	conn, err := net.Dial(network, addr)
	if err != nil {
		return err
	}

	return MakeRequestToConn(conn, signal, params, f)
}

func MakeRequestToConn(conn net.Conn, signal byte, params []byte, f func(conn net.Conn) error) error {
	var buf []byte
	buf = append(buf, Magic...)
	buf = append(buf, signal)
	buf = append(buf, params...)

	if _, err := conn.Write(buf); err != nil {
		return err
	}

	return f(conn)
}
