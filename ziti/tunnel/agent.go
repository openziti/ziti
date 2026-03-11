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

package tunnel

import (
	"bufio"
	"encoding/json"
	"net"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/sdk-golang/inspect"
	"github.com/openziti/sdk-golang/ziti"
	cmap "github.com/orcaman/concurrent-map/v2"
)

const (
	AgentAppId byte = 3
	AgentDump  byte = 128
)

var tunnelContexts = cmap.New[ziti.Context]()

func RegisterContext(name string, ctx ziti.Context) {
	tunnelContexts.Set(name, ctx)
}

func handleAgentDump(conn net.Conn) error {
	c := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

	var results []*inspect.ContextInspectResult
	tunnelContexts.IterCb(func(key string, ctx ziti.Context) {
		results = append(results, ctx.Inspect())
	})

	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		pfxlog.Logger().WithError(err).Error("failed to marshal tunnel dump")
		_, _ = c.WriteString("error: " + err.Error() + "\n")
		return c.Flush()
	}

	_, _ = c.Write(data)
	_, _ = c.WriteString("\n")
	return c.Flush()
}
