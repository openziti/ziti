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
	"github.com/openziti/ziti-db-explorer/cmd/ziti-db-explorer/zdecli"
	"log"
	"os"
)

func main() {
	command := ""

	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	if err := zdecli.Run("", command); err != nil {
		log.Printf("Error: %s", err)
		zdecli.PrintUsage()
		os.Exit(-1)
	} else {
		os.Exit(0)
	}
}
