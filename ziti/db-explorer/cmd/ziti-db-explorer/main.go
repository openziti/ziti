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
package main

import (
	"github.com/c-bata/go-prompt"
	"log"
	"os"
	"strings"
	"ziti-db-explorer/cmd/ziti-db-explorer/zdecli"
	"ziti-db-explorer/zdelib"
)

func main() {
	if len(os.Args) != 2 {
		log.Printf("Error: Invalid number of arguments, got %d expected %d", len(os.Args), 2)
		zdecli.PrintUsage()
		os.Exit(-1)
	}

	if os.Args[1] == "help" {
		zdecli.PrintUsage()
		os.Exit(0)
	}

	if os.Args[1] == "version" {
		zdecli.PrintVersion()
		os.Exit(0)
	}

	log.Printf("opening db file: %s", os.Args[1])

	registry := zdecli.NewCommandRegistry()

	registry.Add(zdecli.CmdQuit, func(state *zdelib.State, _ *zdecli.CommandRegistry, _ string) error {
		os.Exit(0)
		return nil
	})

	registry.Add(zdecli.CmdList, zdecli.ListCurrentBucket)
	registry.Add(zdecli.CmdListAll, zdecli.ListCurrentBucketAll)
	registry.Add(zdecli.CmdCd, zdecli.CdBucket)
	registry.Add(zdecli.CmdCount, zdecli.PrintCurrentCount)
	registry.Add(zdecli.CmdBack, zdecli.NavBackOne)
	registry.Add(zdecli.CmdRoot, zdecli.NavToRoot)
	registry.Add(zdecli.CmdPwd, zdecli.PrintPath)
	registry.Add(zdecli.CmdStatsBucket, zdecli.PrintBucketStats)
	registry.Add(zdecli.CmdStatsDb, zdecli.PrintDbStats)
	registry.Add(zdecli.CmdClear, zdecli.ClearConsole)
	registry.Add(zdecli.CmdShow, zdecli.PrintValue)
	registry.Add(zdecli.CmdHelp, zdecli.PrintHelp)

	state, err := zdelib.NewState(os.Args[1])

	if err != nil {
		log.Printf("Error: %v", err)
		zdecli.PrintUsage()
		os.Exit(-1)
	}

	defer state.Done()

	completer := &zdecli.StateCompleter{
		State:    state,
		Registry: registry,
	}

	for true {
		input := prompt.Input(zdecli.PathPrompt(state)+" ", completer.Complete, prompt.OptionPrefixTextColor(prompt.Cyan))
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
}
