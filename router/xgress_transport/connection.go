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

package xgress_transport

import (
	"github.com/openziti/foundation/transport"
)

type transportXgressConn struct {
	transport.Connection
}

func (c *transportXgressConn) LogContext() string {
	return c.Detail().String()
}

func (c *transportXgressConn) ReadPayload() ([]byte, map[uint8][]byte, error) {
	buffer := make([]byte, 10240)
	n, err := c.Reader().Read(buffer)
	if err == nil {
		if len(buffer) < (5 * 1024) {
			buffer = append([]byte(nil), buffer[:n]...)
		}
	}
	return buffer[:n], nil, err
}

func (c *transportXgressConn) Write(p []byte) (n int, err error) {
	return c.Writer().Write(p)
}

func (c *transportXgressConn) WritePayload(p []byte, _ map[uint8][]byte) (n int, err error) {
	return c.Writer().Write(p)
}
