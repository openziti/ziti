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

package chaos

import (
	"fmt"
	"github.com/openziti/fablab/kernel/model"
	"math/rand"
)

func StaticNumber(val int) func(int) int {
	return func(int) int {
		return val
	}
}
func RandomOfTotal() func(count int) int {
	return func(count int) int {
		return rand.Intn(count) + 1
	}
}

func Percentage(pct uint8) func(count int) int {
	adjustedPct := float64(pct) / 100
	return func(count int) int {
		return int(float64(count) * adjustedPct)
	}
}

func SelectRandom(run model.Run, selector string, f func(count int) int) ([]*model.Component, error) {
	list := run.GetModel().SelectComponents(selector)
	toSelect := f(len(list))

	if toSelect < 1 {
		return nil, nil
	}

	rand.Shuffle(len(list), func(i, j int) {
		list[i], list[j] = list[j], list[i]
	})

	var result []*model.Component
	for i := 0; i < len(list) && i < toSelect; i++ {
		result = append(result, list[i])
	}
	return result, nil
}

func RestartSelected(run model.Run, list []*model.Component, concurrency int) error {
	if len(list) == 0 {
		return nil
	}
	return run.GetModel().ForEachComponentIn(list, concurrency, func(c *model.Component) error {
		if sc, ok := c.Type.(model.ServerComponent); ok {
			if err := c.Type.Stop(run, c); err != nil {
				return err
			}
			return sc.Start(run, c)
		}
		return fmt.Errorf("component %v isn't of ServerComponent type, is of type %T", c, c.Type)
	})
}
