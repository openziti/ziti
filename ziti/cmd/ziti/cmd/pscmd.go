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

package cmd

import (
	"errors"
	"fmt"
	"github.com/openziti/foundation/agent"
	"io"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/openziti/ziti/ziti/util"
)

func trace(addr net.TCPAddr, _ []string) error {
	fmt.Println("Tracing now, will take 5 secs...")
	out, err := cmd(addr, agent.Trace)
	if err != nil {
		return err
	}
	if len(out) == 0 {
		return errors.New("nothing has traced")
	}
	tmpfile, err := ioutil.TempFile("", "trace")
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(tmpfile.Name(), out, 0); err != nil {
		return err
	}
	fmt.Printf("Trace dump saved to: %s\n", tmpfile.Name())
	// If go tool chain not found, stopping here and keep trace file.
	if _, err := exec.LookPath("go"); err != nil {
		return nil
	}
	defer os.Remove(tmpfile.Name())
	cmd := exec.Command("go", "tool", "trace", tmpfile.Name())
	cmd.Env = os.Environ()
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// targetToAddr tries to parse the target string, be it remote host:port
// or local process's PID.
func targetToAddr(target string) (*net.TCPAddr, error) {
	if strings.Contains(target, ":") {
		// addr host:port passed
		var err error
		addr, err := net.ResolveTCPAddr("tcp", target)
		if err != nil {
			return nil, fmt.Errorf("couldn't parse dst address: %v", err)
		}
		return addr, nil
	}
	// try to find port by pid then, connect to local
	pid, err := strconv.Atoi(target)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse PID: %v", err)
	}
	port, err := util.GetPort(pid)
	if err != nil {
		return nil, fmt.Errorf("couldn't get port for PID %v: %v", pid, err)
	}
	addr, _ := net.ResolveTCPAddr("tcp", "127.0.0.1:"+port)
	return addr, nil
}

func cmd(addr net.TCPAddr, c byte, params ...byte) ([]byte, error) {
	conn, err := cmdLazy(addr, c, params...)
	if err != nil {
		return nil, fmt.Errorf("couldn't get port by PID: %v", err)
	}

	all, err := ioutil.ReadAll(conn)
	if err != nil {
		return nil, err
	}
	return all, nil
}

func cmdLazy(addr net.TCPAddr, c byte, params ...byte) (io.Reader, error) {
	conn, err := net.DialTCP("tcp", nil, &addr)
	if err != nil {
		return nil, err
	}
	buf := []byte{c}
	buf = append(buf, params...)
	if _, err := conn.Write(buf); err != nil {
		return nil, err
	}
	return conn, nil
}
