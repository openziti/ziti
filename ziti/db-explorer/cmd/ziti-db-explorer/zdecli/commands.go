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
	"strings"
	"ziti-db-explorer/zdelib"
)

var CmdQuit = &Command{"quit", []string{"q"}, "leave this horrible place", nil}
var CmdList = &Command{"list", []string{"ls"}, "list keys", ListSuggester}
var CmdListAll = &Command{"list-all", []string{"la"}, "list all keys", nil}
var CmdCd = &Command{"cd", nil, "enter a bucket", KeySuggester}
var CmdCount = &Command{"count", nil, "number of keys in bucket", nil}
var CmdBack = &Command{"back", []string{"b"}, "go back one bucket level (alias b)", nil}
var CmdRoot = &Command{"root", []string{"r"}, "return to the root node (alias r)", nil}
var CmdPwd = &Command{"pwd", nil, "print the full path", nil}
var CmdStatsBucket = &Command{"stats-bucket", nil, "show stats for the current bucket", nil}
var CmdStatsDb = &Command{"stats-db", nil, "show stats for the db", nil}
var CmdClear = &Command{"clear", []string{"cls"}, "clear the console", nil}
var CmdShow = &Command{"show", nil, "print the full value of a key", KeySuggester}
var CmdHelp = &Command{"help", nil, "prints help", nil}

// Command represents a string (`Text`) an aliases (`Aliases`) that have a specific description and suggestion
// result set.
type Command struct {
	Text        string
	Aliases     []string
	Description string
	Suggester   func(state *zdelib.State, d prompt.Document) []prompt.Suggest
}

// Matches returns true if the provided text match a command's text or aliases
func (command *Command) Matches(text string) bool {
	if text == command.Text {
		return true
	}

	for _, alias := range command.Aliases {
		if text == alias {
			return true
		}
	}
	return false
}

// Suggest will return the prompt suggestions for the command if command.Suggester is defined. Otherwise, returns nil.
func (command *Command) Suggest(state *zdelib.State, d prompt.Document) []prompt.Suggest {
	if command.Suggester == nil {
		return nil
	}

	return command.Suggester(state, d)
}

const (
	ArgSkip  = "--skip"
	ArgLimit = "--limit"
)

func KeySuggester(state *zdelib.State, _ prompt.Document) []prompt.Suggest {
	var suggestions []prompt.Suggest
	for _, entry := range state.ListEntries() {
		suggestions = append(suggestions, prompt.Suggest{Text: entry.Name})
	}

	return suggestions
}

// ListSuggester returns a list of suggestions for the `list` command
func ListSuggester(_ *zdelib.State, d prompt.Document) []prompt.Suggest {
	var suggestions []prompt.Suggest

	lastSpace1st := strings.LastIndexByte(d.Text, ' ')
	prevWord := ""
	if lastSpace1st != -1 {
		lastSpace2nd := strings.LastIndexByte(d.Text[0:lastSpace1st], ' ')
		if lastSpace2nd == -1 {
			lastSpace2nd = 0
		}
		prevWord = d.Text[lastSpace2nd:lastSpace1st]
	}

	if prevWord == " --skip" || prevWord == " --limit" {
		suggestions = []prompt.Suggest{
			{
				Text:        "<n>",
				Description: "an integer",
			},
		}
	} else {
		if !strings.Contains(d.Text, ArgSkip) {
			suggestions = append(suggestions, prompt.Suggest{
				Text:        ArgSkip,
				Description: ArgSkip + " <n> skips n keys",
			})
		}

		if !strings.Contains(d.Text, ArgLimit) {
			suggestions = append(suggestions, prompt.Suggest{
				Text:        ArgLimit,
				Description: ArgLimit + " <n> limits the results n keys (-1=no limit)",
			})
		}
	}

	return suggestions
}
