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
	"io"
	"net"
	"time"

	"github.com/openziti/channel/v4"
	"github.com/openziti/identity"
	"github.com/openziti/ziti/v2/common/agentid"
	"github.com/pkg/errors"
)

// HandleChannelConnection reads and validates the leading app-id byte on an
// accepted agent connection, then upgrades the connection to an "agent"
// channel bound with the given handler. It is the shared implementation behind
// each application's CustomOpAsync handler.
func HandleChannelConnection(conn net.Conn, id *identity.TokenId, appId byte, bind channel.BindHandler) error {
	appIdBuf := []byte{0}
	if _, err := io.ReadFull(conn, appIdBuf); err != nil {
		return err
	}
	if got := appIdBuf[0]; got != agentid.AppIdAny && got != appId {
		return errors.Errorf("invalid app id %v", got)
	}

	// When log-level callbacks are registered, bind the v2 log-level handlers
	// on every agent channel alongside the application's own handlers.
	bindHandler := bind
	if cbs := getLogLevelCallbacks(); cbs != nil {
		bindHandler = composeBindHandlers(bind, logLevelBindHandler(cbs))
	}

	options := channel.DefaultOptions()
	options.ConnectTimeout = time.Second
	helloHeaders := map[int32][]byte{
		CapabilitiesHeader: GetAgentCapabilitiesMask().Bytes(),
	}
	listener := channel.NewExistingConnListener(id, conn, helloHeaders)
	_, err := channel.NewChannel("agent", listener, bindHandler, options)
	return err
}

// composeBindHandlers returns a BindHandler that applies each of the given
// handlers in order, skipping nil entries.
func composeBindHandlers(handlers ...channel.BindHandler) channel.BindHandler {
	return channel.BindHandlerF(func(binding channel.Binding) error {
		for _, h := range handlers {
			if h == nil {
				continue
			}
			if err := binding.Bind(h); err != nil {
				return err
			}
		}
		return nil
	})
}

// NewChannel wraps an established agent connection in a channel/v4 client
// channel, for callers issuing channel-based agent commands.
func NewChannel(conn net.Conn) (channel.Channel, error) {
	options := channel.DefaultOptions()
	options.ConnectTimeout = time.Second
	dialer := channel.NewExistingConnDialer(&identity.TokenId{Token: "agent"}, conn, nil)
	return channel.NewChannel("agent", dialer, nil, options)
}

// ConnToChannel adapts a channel-consuming function to a conn-consuming one by
// upgrading the connection to an agent channel first.
func ConnToChannel(f func(ch channel.Channel) error) func(conn net.Conn) error {
	return func(conn net.Conn) error {
		ch, err := NewChannel(conn)
		if err != nil {
			return err
		}
		return f(ch)
	}
}

// MakeChannelRequest dials the agent at addr, sends the channel-upgrade op for
// the given appId, and runs f against the resulting channel.
func MakeChannelRequest(addr string, appId byte, f func(ch channel.Channel) error) error {
	return MakeRequestF(addr, CustomOpAsync, []byte{appId}, ConnToChannel(f))
}
