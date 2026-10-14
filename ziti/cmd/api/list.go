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

package api

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"reflect"
	"strings"

	"github.com/Jeffail/gabs"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/openziti/foundation/v2/errorz"
	"github.com/openziti/ziti/v2/ziti/util"
	"github.com/pkg/errors"
)

// ListEntitiesOfType queries the Ziti Controller for entities of the given type
func ListEntitiesWithOptions(api util.API, entityType string, options *Options) ([]*gabs.Container, *Paging, error) {
	params := url.Values{}
	if len(options.Args) > 0 {
		params.Add("filter", options.Args[0])
	}

	return ListEntitiesOfType(api, entityType, params, options.OutputJSONResponse, options.Out, options.Timeout, options.Verbose)
}

// ListEntitiesOfType queries the Ziti Controller for entities of the given type
func ListEntitiesOfType(api util.API, entityType string, params url.Values, logJSON bool, out io.Writer, timeout int, verbose bool) ([]*gabs.Container, *Paging, error) {
	jsonParsed, err := util.ControllerList(api, entityType, params, logJSON, out, timeout, verbose)

	if err != nil {
		return nil, nil, err
	}

	children, err := jsonParsed.S("data").Children()
	return children, GetPaging(jsonParsed), err
}

func GetPaging(c *gabs.Container) *Paging {
	pagingInfo := &Paging{}
	pagination := c.S("meta", "pagination")
	if pagination != nil {
		pagingInfo.Limit = toInt64(pagination, "limit", pagingInfo)
		pagingInfo.Offset = toInt64(pagination, "offset", pagingInfo)
		pagingInfo.Count = toInt64(pagination, "totalCount", pagingInfo)
	} else {
		pagingInfo.SetError(errors.New("meta.pagination section not found in result"))
	}
	return pagingInfo
}

type Paging struct {
	Limit  int64
	Offset int64
	Count  int64
	errorz.ErrorHolderImpl
}

func (p *Paging) Output(o *Options) {
	if p.HasError() {
		_, _ = fmt.Fprintf(o.Out, "unable to retrieve paging information: %v\n", p.Err)
	} else if p.Count == 0 {
		_, _ = fmt.Fprintln(o.Out, "results: none")
	} else {
		first := p.Offset + 1
		last := p.Offset + p.Limit
		if last > p.Count || last < 0 { // if p.Limit is maxint, last will rollover and be negative
			last = p.Count
		}
		_, _ = fmt.Fprintf(o.Out, "results: %v-%v of %v\n", first, last, p.Count)
	}
}

func toInt64(c *gabs.Container, path string, errorHolder errorz.ErrorHolder) int64 {
	data := c.S(path).Data()
	if data == nil {
		errorHolder.SetError(errors.Errorf("%v not found", path))
		return 0
	}
	val, ok := data.(float64)
	if !ok {
		errorHolder.SetError(errors.Errorf("%v not a number, it's a %v", path, reflect.TypeOf(data)))
		return 0
	}
	return int64(val)
}

func FilterEntitiesOfType(api util.API, entityType string, filter string, logJSON bool, out io.Writer, timeout int, verbose bool) ([]*gabs.Container, *Paging, error) {
	params := url.Values{}
	params.Add("filter", filter)
	return ListEntitiesOfType(api, entityType, params, logJSON, out, timeout, verbose)
}

// MapEdgeNameToID resolves idOrName to the id of one entityType entity in the edge management API, as
// MapEdgeNamesToIDs does, failing if it matches none or several.
func MapEdgeNameToID(entityType string, o *Options, idOrName string) (string, error) {
	return mapNameToID(util.EdgeAPI, entityType, o, idOrName)
}

// MapEdgeNamesToIDs resolves each value of list to the id of an entityType entity in the edge management API. A value
// prefixed "id:" is taken as an id without a lookup, one prefixed "name:" matches by name only, and any other value
// matches by id or name. A value matching several entities is an error. A value matching none is an error, unless
// skipNotFound is set, in which case it is reported on o.Out and left out of the result. Unless skipNotFound is set,
// the result holds one id per value of list, in order.
func MapEdgeNamesToIDs(entityType string, o *Options, skipNotFound bool, list ...string) ([]string, error) {
	return mapNamesToIDs(util.EdgeAPI, entityType, o, skipNotFound, list...)
}

// MapFabricNameToID is MapEdgeNameToID for the fabric management API.
func MapFabricNameToID(entityType string, o *Options, idOrName string) (string, error) {
	return mapNameToID(util.FabricAPI, entityType, o, idOrName)
}

// MapFabricNamesToIDs is MapEdgeNamesToIDs for the fabric management API.
func MapFabricNamesToIDs(entityType string, o *Options, skipNotFound bool, list ...string) ([]string, error) {
	return mapNamesToIDs(util.FabricAPI, entityType, o, skipNotFound, list...)
}

func mapNameToID(api util.API, entityType string, o *Options, idOrName string) (string, error) {
	ids, err := mapNamesToIDs(api, entityType, o, false, idOrName)
	if err != nil {
		return "", err
	}
	return ids[0], nil
}

// mapNamesToIDs implements MapEdgeNamesToIDs and MapFabricNamesToIDs against api.
func mapNamesToIDs(api util.API, entityType string, o *Options, skipNotFound bool, list ...string) ([]string, error) {
	var result []string
	for _, val := range list {
		id, filter := parseIdOrName(val)
		if filter == "" {
			result = append(result, id)
			continue
		}

		entities, _, err := FilterEntitiesOfType(api, entityType, filter, false, nil, o.Timeout, o.Verbose)
		if err != nil {
			return nil, err
		}

		if len(entities) == 0 {
			if skipNotFound {
				o.Printf("no %v found matching %v\n", entityType, val)
				continue
			}
			return nil, errors.Errorf("no %v found matching %v", entityType, val)
		}

		if len(entities) > 1 {
			return nil, errors.Errorf("multiple %v match %v, prefix it with id: or name: to choose", entityType, val)
		}

		entityId, _ := entities[0].Path("id").Data().(string)
		if debug, found := os.LookupEnv("ZITI_CLI_DEBUG"); found && strings.EqualFold("true", debug) {
			fmt.Printf("Found %v with id %v for %v\n", entityType, entityId, val)
		}
		result = append(result, entityId)
	}
	return result, nil
}

// parseIdOrName returns the id named by an "id:"-prefixed val, with an empty filter. Otherwise it returns the filter
// that finds the entity val refers to: by name for a "name:" prefix, and by id or name for anything else.
func parseIdOrName(val string) (id string, filter string) {
	if id, ok := strings.CutPrefix(val, "id:"); ok {
		return id, ""
	}
	if name, ok := strings.CutPrefix(val, "name:"); ok {
		return "", fmt.Sprintf(`name="%s"`, name)
	}
	return "", fmt.Sprintf(`id="%s" or name="%s"`, val, val)
}

func RenderTable(o *Options, t table.Writer, pagingInfo *Paging) {
	if o.OutputCSV {
		if _, err := fmt.Fprintln(o.Cmd.OutOrStdout(), t.RenderCSV()); err != nil {
			panic(err)
		}
	} else {
		if _, err := fmt.Fprintln(o.Cmd.OutOrStdout(), t.Render()); err != nil {
			panic(err)
		}
		if pagingInfo != nil {
			pagingInfo.Output(o)
		}
	}
}
