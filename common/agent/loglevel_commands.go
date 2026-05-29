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
	"fmt"
	"time"

	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v4"
	"github.com/openziti/ziti/v2/common/handler_common"
	"github.com/pkg/errors"
)

// Content-type IDs for the v2 log-level channel messages. These ride the same
// "agent" channel as application message types (ctrl_pb in the 1000s, mgmt_pb
// in the 10000s), so common/agent reserves the 30000-30999 band for its own
// messages.
const (
	SetLogLevelV2RequestType          int32 = 30000
	SetChannelLogLevelV2RequestType   int32 = 30001
	ClearChannelLogLevelV2RequestType int32 = 30002
)

// String header IDs carrying the v2 log-level message parameters, in the
// agent-reserved band.
const (
	LogLevelHeader   int32 = 30100
	LogChannelHeader int32 = 30101
)

// LogLevelCallbacks holds the application-supplied side effects for the
// log-level commands. The application maps the agent-neutral LogLevel onto its
// own loggers. The struct shape leaves room to add fields later without
// breaking callers.
type LogLevelCallbacks struct {
	SetLogLevel          func(level LogLevel)
	SetChannelLogLevel   func(channel string, level LogLevel)
	ClearChannelLogLevel func(channel string)
}

// logLevelCallbacks holds the registered callbacks, guarded by capsMu. It is
// nil until RegisterLogLevelHandlers is called.
var logLevelCallbacks *LogLevelCallbacks

// RegisterLogLevelHandlers registers the application's log-level side effects.
// All three callbacks must be supplied. The first call advertises
// logging.slog-levels and binds the v2 log-level channel commands on every
// subsequent agent channel; the framed log-level commands route through the
// same callbacks. Subsequent calls replace the callbacks in place, so multiple
// in-process apps (e.g. the quickstart's controller and router) can share the
// agent listener without fighting over registration order. The first call must
// happen before the agent listener starts; later first-time registration panics
// because it would add a new capability bit and the advertised set must stay
// consistent across every connection.
func RegisterLogLevelHandlers(cbs LogLevelCallbacks) error {
	if cbs.SetLogLevel == nil || cbs.SetChannelLogLevel == nil || cbs.ClearChannelLogLevel == nil {
		return errors.New("all log-level callbacks must be set")
	}

	capsMu.Lock()
	defer capsMu.Unlock()
	if !agentCapAlreadyActive(CapabilityLoggingSlogLevels) {
		// First registration: the call would advertise a new bit, so the freeze
		// rule applies. Subsequent calls only refresh callbacks and pass through.
		assertCapsMutable("RegisterLogLevelHandlers")
	}
	cb := cbs
	logLevelCallbacks = &cb
	activeCaps[CapabilityLoggingSlogLevels] = true
	return nil
}

// getLogLevelCallbacks returns the registered callbacks, or nil if none were
// registered.
func getLogLevelCallbacks() *LogLevelCallbacks {
	capsMu.Lock()
	defer capsMu.Unlock()
	return logLevelCallbacks
}

// logLevelBindHandler binds the v2 log-level message handlers onto an agent
// channel. HandleChannelConnection adds it automatically when callbacks are
// registered.
func logLevelBindHandler(cbs *LogLevelCallbacks) channel.BindHandler {
	return channel.BindHandlerF(func(binding channel.Binding) error {
		binding.AddReceiveHandlerF(SetLogLevelV2RequestType, func(m *channel.Message, ch channel.Channel) {
			levelStr, _ := m.GetStringHeader(LogLevelHeader)
			level, err := ParseLogLevel(levelStr)
			if err != nil {
				handler_common.SendOpResult(m, ch, "set-log-level", err.Error(), false)
				return
			}
			cbs.SetLogLevel(level)
			pfxlog.Logger().Infof("log level set to %v", level)
			handler_common.SendOpResult(m, ch, "set-log-level", "log level set to "+level.String(), true)
		})

		binding.AddReceiveHandlerF(SetChannelLogLevelV2RequestType, func(m *channel.Message, ch channel.Channel) {
			channelName, _ := m.GetStringHeader(LogChannelHeader)
			levelStr, _ := m.GetStringHeader(LogLevelHeader)
			level, err := ParseLogLevel(levelStr)
			if err != nil {
				handler_common.SendOpResult(m, ch, "set-channel-log-level", err.Error(), false)
				return
			}
			cbs.SetChannelLogLevel(channelName, level)
			pfxlog.Logger().Infof("log level for channel %v set to %v", channelName, level)
			handler_common.SendOpResult(m, ch, "set-channel-log-level", fmt.Sprintf("log level for channel %v set to %v", channelName, level), true)
		})

		binding.AddReceiveHandlerF(ClearChannelLogLevelV2RequestType, func(m *channel.Message, ch channel.Channel) {
			channelName, _ := m.GetStringHeader(LogChannelHeader)
			cbs.ClearChannelLogLevel(channelName)
			pfxlog.Logger().Infof("log level for channel %v cleared", channelName)
			handler_common.SendOpResult(m, ch, "clear-channel-log-level", "log level for channel "+channelName+" cleared", true)
		})

		return nil
	})
}

// SendSetLogLevelV2 sends a SetLogLevelV2 request over the given agent channel
// and returns the server's result message.
func SendSetLogLevelV2(ch channel.Channel, level LogLevel, timeout time.Duration) (string, error) {
	msg := channel.NewMessage(SetLogLevelV2RequestType, nil)
	msg.PutStringHeader(LogLevelHeader, level.String())
	return sendForResult(ch, msg, timeout)
}

// SendSetChannelLogLevelV2 sends a SetChannelLogLevelV2 request over the given
// agent channel and returns the server's result message.
func SendSetChannelLogLevelV2(ch channel.Channel, channelName string, level LogLevel, timeout time.Duration) (string, error) {
	msg := channel.NewMessage(SetChannelLogLevelV2RequestType, nil)
	msg.PutStringHeader(LogChannelHeader, channelName)
	msg.PutStringHeader(LogLevelHeader, level.String())
	return sendForResult(ch, msg, timeout)
}

// SendClearChannelLogLevelV2 sends a ClearChannelLogLevelV2 request over the
// given agent channel and returns the server's result message.
func SendClearChannelLogLevelV2(ch channel.Channel, channelName string, timeout time.Duration) (string, error) {
	msg := channel.NewMessage(ClearChannelLogLevelV2RequestType, nil)
	msg.PutStringHeader(LogChannelHeader, channelName)
	return sendForResult(ch, msg, timeout)
}

// sendForResult sends a request and waits for the standard channel Result
// reply, returning its message on success or an error on transport failure or
// a non-success result.
func sendForResult(ch channel.Channel, msg *channel.Message, timeout time.Duration) (string, error) {
	reply, err := msg.WithTimeout(timeout).SendForReply(ch)
	if err != nil {
		return "", err
	}
	if reply.ContentType != channel.ContentTypeResultType {
		return "", errors.Errorf("unexpected response type %v", reply.ContentType)
	}
	result := channel.UnmarshalResult(reply)
	if !result.Success {
		return "", errors.New(result.Message)
	}
	return result.Message, nil
}
