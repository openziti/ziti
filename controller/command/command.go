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

package command

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/channel/v2"
	"github.com/openziti/fabric/controller/change"
	"github.com/openziti/foundation/v2/debugz"
	"github.com/openziti/storage/boltz"
	"github.com/sirupsen/logrus"
	"reflect"
)

// Command instances represent actions to be taken by the fabric controller. They are serializable,
// so they can be shipped from one controller for RAFT coordination
type Command interface {
	// Apply runs the commands
	Apply(ctx boltz.MutateContext) error

	// GetChangeContext returns the change context associated with the command
	GetChangeContext() *change.Context

	// Encode returns a serialized representation of the command
	Encode() ([]byte, error)
}

// Validatable instances can be validated. Command instances which implement Validable will be validated
// before Command.Apply is called
type Validatable interface {
	Validate() error
}

// Dispatcher instances will take a command and either send it to the leader to be applied, or if the current
// system is the leader, apply it locally
type Dispatcher interface {
	Dispatch(command Command) error
	IsLeaderOrLeaderless() bool
	GetPeers() map[string]channel.Channel
}

// LocalDispatcher should be used when running a non-clustered system
type LocalDispatcher struct {
	EncodeDecodeCommands bool
}

func (self *LocalDispatcher) IsLeaderOrLeaderless() bool {
	return true
}

func (self *LocalDispatcher) GetPeers() map[string]channel.Channel {
	return nil
}

func (self *LocalDispatcher) Dispatch(command Command) error {
	defer func() {
		if p := recover(); p != nil {
			pfxlog.Logger().
				WithField(logrus.ErrorKey, p).
				WithField("cmdType", reflect.TypeOf(command)).
				Error("error while dispatching command of type")
			debugz.DumpLocalStack()
			panic(p)
		}
	}()

	if self.EncodeDecodeCommands {
		bytes, err := command.Encode()
		if err != nil {
			return err
		}
		cmd, err := GetDefaultDecoders().Decode(bytes)
		if err != nil {
			return err
		}
		return cmd.Apply(change.New().NewMutateContext())
	}

	return command.Apply(change.New().NewMutateContext())
}

// Decoder instances know how to decode encoded commands
type Decoder interface {
	Decode(commandType int32, data []byte) (Command, error)
}

// DecoderF is a function version of the Decoder interface
type DecoderF func(commandType int32, data []byte) (Command, error)

func (self DecoderF) Decode(commandType int32, data []byte) (Command, error) {
	return self(commandType, data)
}
