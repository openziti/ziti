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
package zdecli

import (
	"errors"
	"github.com/c-bata/go-prompt"
	"sort"
	"ziti-db-explorer/zdelib"
)

// ActionHandler is a function which can take an unparsed CLI string, parse it, and process an action
type ActionHandler func(state *zdelib.State, args string) error

// Action is prompt.Suggest with additional values to control suggestion listing as well as to couple it to an
// ActionHandler.
type Action struct {
	prompt.Suggest
	Do          ActionHandler
	IsSuggested bool
}

// NewAction creates a default action with an ActionHandler that will simply print a message.
func NewAction(text, desc string, do ActionHandler) *Action {
	if do == nil {
		do = func(state *zdelib.State, _ string) error {
			return errors.New("nothing to do")
		}
	}
	return &Action{
		Suggest: prompt.Suggest{
			Text:        text,
			Description: desc,
		},
		IsSuggested: false,
		Do:          do,
	}
}

// CommandRegistry provides efficient lookup of Action and Command entities by command text and aliases for
// suggestions and action invocation.
type CommandRegistry struct {
	CommandTextToCommand map[string]*Command
	CommandTextToAction  map[string]*Action
	CommandTexts         []string
}

// NewCommandRegistry returns an empty registry.
func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{
		CommandTextToCommand: map[string]*Command{},
		CommandTextToAction:  map[string]*Action{},
		CommandTexts:         []string{},
	}
}

// Add adds a new string cmdText with any number of string command aliases. `desc` will be displayed to provide hints
// on what a command does. The provided ActionHandler will be invoked when cmdText or aliases are matched.
func (registry *CommandRegistry) Add(cmd *Command, handler ActionHandler) {
	registry.add(cmd, handler, true)
}

// AddUnlisted is the same as Add, but it will not show up in suggestions.
func (registry *CommandRegistry) AddUnlisted(cmd *Command, handler ActionHandler) {
	registry.add(cmd, handler, false)
}

// add is an internal function used to power Add and AddUnlisted.
func (registry *CommandRegistry) add(cmd *Command, handler ActionHandler, isSuggested bool) {
	registry.CommandTextToCommand[cmd.Text] = cmd
	registry.CommandTextToAction[cmd.Text] = NewAction(cmd.Text, cmd.Description, handler)
	registry.CommandTextToAction[cmd.Text].IsSuggested = isSuggested
	registry.CommandTexts = append(registry.CommandTexts, cmd.Text)

	for _, alias := range cmd.Aliases {
		registry.CommandTextToCommand[alias] = cmd
		registry.CommandTextToAction[alias] = NewAction(alias, cmd.Description, handler)
		registry.CommandTexts = append(registry.CommandTexts, alias)
	}

	sort.Strings(registry.CommandTexts)
}

// GetCommand returns the command with text or aliases that match cmdText. If not found, returns nil
func (registry *CommandRegistry) GetCommand(cmdText string) *Command {
	cmd, _ := registry.CommandTextToCommand[cmdText]

	return cmd
}
