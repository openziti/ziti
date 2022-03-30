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

package ast

import (
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/storage/zitiql"
	"github.com/openziti/foundation/util/concurrenz"
)

var EnableQueryDebug concurrenz.AtomicBoolean

func Parse(symbolTypes SymbolTypes, query string) (Query, error) {
	if EnableQueryDebug.Get() {
		pfxlog.Logger().Debugf(`parsing filter: %v`, query)
	}
	listener := NewListener()

	if query == "" {
		return &queryNode{
			Predicate: BoolNodeTrue,
			SortBy:    &SortByNode{},
		}, nil
	}

	parseErrors := zitiql.Parse(query, listener)
	if len(parseErrors) != 0 {
		return nil, parseErrors[0]
	}

	return listener.getQuery(symbolTypes)
}

func PostProcess(symbolTypes SymbolTypes, node *BoolNode) error {
	setSymbolValidator := &SymbolValidator{symbolTypes: symbolTypes}
	(*node).Accept(setSymbolValidator)
	if setSymbolValidator.HasError() {
		return setSymbolValidator.GetError()
	}

	return transformBools(symbolTypes, node)
}

func transformBools(s SymbolTypes, nodes ...*BoolNode) error {
	for _, node := range nodes {
		if sp, ok := (*node).(BoolTypeTransformable); ok {
			transformed, err := sp.TypeTransformBool(s)
			if err != nil {
				return err
			}
			*node = transformed
		}
	}
	return nil
}
