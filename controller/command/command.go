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

// Command instances represent actions to be taken by the fabric controller. They are serializable,
// so they can be shipped from one controller for RAFT coordination
type Command interface {
	// Apply runs the commands
	Apply() error

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
}

// LocalDispatcher should be used when running a non-clustered system
type LocalDispatcher struct{}

func (LocalDispatcher) Dispatch(command Command) error {
	return command.Apply()
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
