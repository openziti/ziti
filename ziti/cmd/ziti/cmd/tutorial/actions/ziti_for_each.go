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
	"github.com/openziti/ziti/ziti/cmd/ziti/cmd/api"
	"github.com/openziti/ziti/ziti/cmd/ziti/tutorial"
	"github.com/openziti/ziti/ziti/cmd/ziti/util"
	"github.com/pkg/errors"
	"net/url"
	"os"
	"strconv"
)

type ZitiForEach struct{}

func (self *ZitiForEach) Execute(ctx *tutorial.ActionContext) error {
	ctx.Headers["templatize"] = "true"
	apiType, found := ctx.Headers["api"]
	if !found {
		apiType = string(util.EdgeAPI)
	}
	entityType := ctx.Headers["type"]

	params := url.Values{}
	filter, found := ctx.Headers["filter"]
	if found {
		params.Add("filter", filter)
	}

	minCount := 1
	if count, found := ctx.Headers["minCount"]; found {
		val, err := strconv.Atoi(count)
		if err != nil {
			return errors.Wrapf(err, "couldn't parse minCount, invalid value '%v'", count)
		}
		if val < 0 {
			return errors.Wrapf(err, "invalid minCount, invalid value '%v', must >= 0", count)
		}
		minCount = val
	}

	maxCount := 1
	if count, found := ctx.Headers["maxCount"]; found {
		val, err := strconv.Atoi(count)
		if err != nil {
			return errors.Wrapf(err, "couldn't parse maxCount, invalid value '%v'", count)
		}
		if val < minCount {
			return errors.Wrapf(err, "invalid maxCount '%v', must >= minCount of %v", count, minCount)
		}
		maxCount = val
	}

	entities, _, err := api.ListEntitiesOfType(util.API(apiType), entityType, params, false, os.Stdout, 1, false)
	if err != nil {
		return err
	}

	if len(entities) < minCount {
		return errors.Errorf("expected at least %v %v, only found %v", minCount, entityType, len(entities))
	}

	if len(entities) > maxCount {
		return errors.Errorf("expected at most %v %v, only found %v", maxCount, entityType, len(entities))
	}

	runner := ZitiRunnerAction{}
	originalBody := ctx.Body
	for _, entity := range entities {
		wrapper := api.Wrap(entity)
		id := wrapper.String("id")
		name := wrapper.String("name")

		ctx.Runner.AddVariable("entityId", id)
		ctx.Runner.AddVariable("entityName", name)
		if err := runner.Execute(ctx); err != nil {
			return err
		}
		ctx.Runner.ClearVariable("entityId")
		ctx.Runner.ClearVariable("entityName")
		ctx.Body = originalBody
	}

	return nil
}
