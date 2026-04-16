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
	"fmt"
	"github.com/c-bata/go-prompt"
	"github.com/openziti/ziti-db-explorer/zdelib"
	"log"
	"os"
	"strings"
)

// CommandName allow usage and such to be altered to fit a hosting executable
var CommandName = "ziti-db-explorer"

// Run is called by main() and is provided so that ziti-db-explorer can be embedded in other CLIs. commandName should
// be the full command to run ziti-db-explorer (e.g. ziti db explore).
func Run(commandName, arg string) error {
	if commandName != "" {
		CommandName = commandName
	}

	if arg == "" {
		return fmt.Errorf("no path or command supplied")
	}

	if arg == "help" {
		PrintUsage()
		return nil
	}

	if arg == "version" {
		PrintVersion()
		return nil
	}

	log.Printf("opening db file: %s", arg)

	registry := NewCommandRegistry()

	registry.Add(CmdQuit, func(state *zdelib.State, _ *CommandRegistry, _ string) error {
		os.Exit(0)
		return nil
	})

	registry.Add(CmdList, ListCurrentBucket)
	registry.Add(CmdListAll, ListCurrentBucketAll)
	registry.Add(CmdCd, CdBucket)
	registry.Add(CmdCount, PrintCurrentCount)
	registry.Add(CmdBack, NavBackOne)
	registry.Add(CmdRoot, NavToRoot)
	registry.Add(CmdPwd, PrintPath)
	registry.Add(CmdStatsBucket, PrintBucketStats)
	registry.Add(CmdStatsDb, PrintDbStats)
	registry.Add(CmdClear, ClearConsole)
	registry.Add(CmdShow, PrintValue)
	registry.Add(CmdHelp, PrintHelp)

	state, err := zdelib.NewState(arg)

	if err != nil {
		log.Printf("Error: %v", err)
		PrintUsage()
		os.Exit(-1)
	}

	defer state.Done()

	completer := &StateCompleter{
		State:    state,
		Registry: registry,
	}

	for true {
		input := prompt.Input(PathPrompt(state)+" ", completer.Complete, prompt.OptionPrefixTextColor(prompt.Cyan))
		input = strings.TrimSpace(input)

		index := strings.IndexByte(input, ' ')
		cmd := input
		args := ""
		if index != -1 {
			cmd = input[0:index]
			args = input[index:]
		}

		if action, ok := registry.CommandTextToAction[cmd]; ok {
			if err := action.Do(state, registry, args); err != nil {
				log.Printf("Error: %v", err)
			}
		} else {
			if strings.HasPrefix(input, "\"") && strings.HasSuffix(input, "\"") {
				input = input[1 : len(input)-1]
			}

			err := state.Enter(input)

			if err != nil {
				log.Printf("Error: unknown command or bucket name")
			}
		}
	}

	return nil
}
