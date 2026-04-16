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
	"github.com/c-bata/go-prompt"
	"github.com/openziti/ziti-db-explorer/zdelib"
	"strings"
)

// StateCompleter is used to package a prompt.Completer with location zdelib.State as well as an CommandRegistry.
// This allows a prompt.Completer to provide current bucket location suggestions (i.e. key values completing) as well
// as provide command suggestions.
type StateCompleter struct {
	State    *zdelib.State
	Registry *CommandRegistry
}

// FirstWord returns the first word of the current input or empty string
func (completer *StateCompleter) FirstWord(d prompt.Document) string {
	if d.Text == "" {
		return ""
	}

	firstSpace := strings.IndexByte(d.Text, ' ')

	if firstSpace == -1 {
		return ""
	}

	return d.Text[0:firstSpace]
}

// Complete implements the signature required of a prompt.Completer. It parses the current prompt.Document to provide
// suggestions and tab-completion for bucket keys as well as Action's from the provided CommandRegistry.
func (completer *StateCompleter) Complete(d prompt.Document) []prompt.Suggest {
	var suggestions []prompt.Suggest

	firstWord := completer.FirstWord(d)

	completer.Registry.GetCommand(firstWord)

	found := false

	for _, cmd := range completer.Registry.CommandTextToCommand {
		if cmd.Matches(firstWord) {
			found = true
			for _, sug := range cmd.Suggest(completer.State, d) {
				suggestions = append(suggestions, sug)
			}
			break
		}
	}

	if !found && strings.IndexByte(d.Text, ' ') == -1 {
		for _, key := range completer.Registry.CommandTexts {
			action := completer.Registry.CommandTextToAction[key]
			if action.IsSuggested {
				suggestions = append(suggestions, action.Suggest)
			}
		}
	}

	return prompt.FilterHasPrefix(suggestions, d.GetWordBeforeCursor(), true)
}
