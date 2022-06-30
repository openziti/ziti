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
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/edge"
	"github.com/openziti/ziti/ziti/cmd/ziti/tutorial"
	"net/url"
	"strings"
	"time"
)

type KeepSessionAliveAction struct{}

func (self *KeepSessionAliveAction) Execute(ctx *tutorial.ActionContext) error {
	interval := time.Minute
	if val, ok := ctx.Headers["interval"]; ok {
		if d, err := time.ParseDuration(val); err != nil {
			return err
		} else {
			interval = d
		}
	}
	if !strings.EqualFold("true", ctx.Headers["quiet"]) {
		fmt.Printf("Running session refresh every %v\n", interval)
	}
	go func() {
		ticker := time.NewTicker(interval)
		for {
			select {
			case <-ticker.C:
				_, _, _ = edge.ListEntitiesOfType("edge-routers", url.Values{}, false, nil, 10, false)
			}
		}
	}()
	return nil
}
