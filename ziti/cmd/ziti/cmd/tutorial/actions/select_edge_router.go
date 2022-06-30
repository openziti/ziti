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

package actions

import (
	"fmt"
	"github.com/openziti/foundation/util/term"
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/edge"
	"github.com/openziti/ziti/ziti/cmd/ziti/tutorial"
	"github.com/pkg/errors"
	"strconv"
	"strings"
)

type edgeRouter struct {
	id   string
	name string
}

type SelectEdgeRouterAction struct{}

func (self *SelectEdgeRouterAction) Execute(ctx *tutorial.ActionContext) error {
	for {
		err := self.SelectEdgeRouter(ctx)
		if err == nil {
			return nil
		}

		retry, err2 := tutorial.AskYesNoWithDefault(fmt.Sprintf("Error getting edge router (err=%v). Try again? [Y/N] (default Y): ", err), true)
		if err2 != nil {
			fmt.Printf("encountered error prompting for input: %v\n", err2)
			return err
		}

		if !retry {
			return err
		}
	}
}

func (self *SelectEdgeRouterAction) SelectEdgeRouter(ctx *tutorial.ActionContext) error {
	fmt.Println("")

	valid := false
	var edgeRouterName string

	for !valid {
		children, _, err := edge.ListEntitiesWithFilter("edge-routers", "limit none")
		if err != nil {
			return errors.Wrap(err, "unable to list edge routers")
		}

		var ers []*edgeRouter

		if len(children) == 0 {
			return errors.New("no edge routers found")
		}

		for _, child := range children {
			isOnline := child.S("isOnline").Data().(bool)
			if isOnline {
				id := child.S("id").Data().(string)
				name := child.S("name").Data().(string)
				ers = append(ers, &edgeRouter{id: id, name: name})
			}
		}

		if len(ers) == 0 {
			fmt.Println("Error: no online edge routers found. Found these offline edge routers: ")
			for _, child := range children {
				id := child.S("id").Data().(string)
				name := child.S("name").Data().(string)
				fmt.Printf("id: %10v name: %10v\n", id, name)
			}
			return errors.New("no on-line edge routers")
		}

		fmt.Printf("Available edge routers: \n\n")
		for idx, er := range ers {
			fmt.Printf("  %v: %v\n", idx+1, er.name)
		}
		fmt.Print("  R: Refresh list from controller\n")
		var val string

		if !ctx.Runner.AssumeDefault {
			val, err = term.Prompt("\nSelect edge router, by number or name (default 1): ")
			if err != nil {
				return err
			}
		}

		if val == "" {
			edgeRouterName = ers[0].name
			valid = true
		} else {
			if idx, err := strconv.Atoi(val); err == nil {
				if idx > 0 && idx <= len(ers) {
					edgeRouterName = ers[idx-1].name
					valid = true
				}
			}
		}

		if !valid {
			for _, er := range ers {
				if val == er.name {
					edgeRouterName = val
					valid = true
				}
			}
		}

		if !valid {
			if strings.EqualFold("r", val) {
				fmt.Println("Refreshing edge router list")
			} else {
				fmt.Printf("Invalid input %v\n", val)
			}
		}
	}

	ctx.Runner.AddVariable("edgeRouterName", edgeRouterName)
	return nil
}
