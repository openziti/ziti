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

package xgress_proxy

import (
	"github.com/openziti/channel"
	"github.com/openziti/fabric/router/xgress"
	"github.com/openziti/transport/v2"
	"github.com/pkg/errors"
)

type proxyXgressConnection struct {
	transport.Conn
}

func (c *proxyXgressConnection) LogContext() string {
	return c.Detail().String()
}

func (c *proxyXgressConnection) ReadPayload() ([]byte, map[uint8][]byte, error) {
	buffer := make([]byte, 10240)
	n, err := c.Read(buffer)
	return buffer[:n], nil, err
}

func (c *proxyXgressConnection) WritePayload(p []byte, headers map[uint8][]byte) (n int, err error) {
	return c.Write(p)
}

func (self *proxyXgressConnection) HandleControlMsg(controlType xgress.ControlType, headers channel.Headers, responder xgress.ControlReceiver) error {
	if controlType == xgress.ControlTypeTraceRoute {
		xgress.RespondToTraceRequest(headers, "xgress/proxy", "", responder)
		return nil
	}
	return errors.Errorf("unhandled control type: %v", controlType)
}
