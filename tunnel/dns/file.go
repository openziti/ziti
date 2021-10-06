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

package dns

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
)

const hostFormat = "%s\t%s\t# NetFoundry"

type hostFile struct {
	path    string
	mutex   sync.Mutex
	domains map[string]*domainEntry
}

func NewHostFile(path string) Resolver {
	return &hostFile{
		path:  path,
		mutex: sync.Mutex{},
	}
}

func isHostPresent(hostname string, ip net.IP, f *os.File) bool {
	scanner := bufio.NewScanner(f)
	match := fmt.Sprintf(hostFormat, ip.String(), hostname)
	for scanner.Scan() {
		if scanner.Text() == match {
			return true
		}
	}
	return false
}

func (h *hostFile) AddDomain(name string, _ func(string) (net.IP, error)) error {
	return fmt.Errorf("cannot add wildcard domain[%s] to hostfile resolver", name)
}

func (h *hostFile) Lookup(_ net.IP) (string, error) {
	return "", fmt.Errorf("not implemented")
}

func (h *hostFile) AddHostname(hostname string, ip net.IP) error {
	h.mutex.Lock()
	defer h.mutex.Unlock()
	f, err := os.OpenFile(h.path, os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	if !isHostPresent(hostname, ip, f) {
		_, err := f.Seek(0, io.SeekEnd)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(f, hostFormat+"\n", ip.String(), hostname)
		if err != nil {
			return err
		}
	}

	return nil
}

func (h *hostFile) RemoveHostname(_ string) error {
	return nil
}

func (h *hostFile) Cleanup() error {
	return nil
}
