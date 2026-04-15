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

package objectz

import (
	"github.com/openziti/storage/ast"
	"time"
)

type ObjectCursor[T any] struct {
	store   *ObjectStore[T]
	current T
}

func (self *ObjectCursor[T]) GetSymbolType(name string) (ast.NodeType, bool) {
	symbol, found := self.store.symbols[name]
	if !found {
		return 0, false
	}
	return symbol.GetType(), true
}

func (self *ObjectCursor[T]) GetSetSymbolTypes(name string) ast.SymbolTypes {
	return nil
}

func (self *ObjectCursor[T]) IsSet(name string) (bool, bool) {
	_, found := self.store.symbols[name]
	return false, found
}

func (self *ObjectCursor[T]) eval(name string) any {
	symbol := self.store.symbols[name]
	result := symbol.Eval(self.current)
	return result
}

func (self *ObjectCursor[T]) EvalBool(name string) *bool {
	if val, ok := self.eval(name).(*bool); ok {
		return val
	}
	return nil
}

func (self *ObjectCursor[T]) EvalString(name string) *string {
	if val, ok := self.eval(name).(*string); ok {
		return val
	}
	return nil
}

func (self *ObjectCursor[T]) EvalInt64(name string) *int64 {
	if val, ok := self.eval(name).(*int64); ok {
		return val
	}
	return nil
}

func (self *ObjectCursor[T]) EvalFloat64(name string) *float64 {
	if val, ok := self.eval(name).(*float64); ok {
		return val
	}
	return nil
}

func (self *ObjectCursor[T]) EvalDatetime(name string) *time.Time {
	if val, ok := self.eval(name).(*time.Time); ok {
		return val
	}
	return nil
}

func (self *ObjectCursor[T]) IsNil(name string) bool {
	return nil == self.eval(name)
}

func (self *ObjectCursor[T]) OpenSetCursor(name string) ast.SetCursor {
	//TODO implement me
	panic("implement me")
}

func (self *ObjectCursor[T]) OpenSetCursorForQuery(name string, query ast.Query) ast.SetCursor {
	//TODO implement me
	panic("implement me")
}
