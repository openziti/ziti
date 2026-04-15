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

package boltz

import (
	"github.com/michaelquigley/pfxlog"
	"time"

	"github.com/openziti/storage/ast"
	"go.etcd.io/bbolt"
)

var _ ast.Symbols = (*rowCursorImpl)(nil)

type rowCursorImpl struct {
	symbolCache map[string]EntitySymbol
	entity      Store
	currentRow  []byte
	tx          *bbolt.Tx
}

func newRowCursor(entity Store, tx *bbolt.Tx) *rowCursorImpl {
	return &rowCursorImpl{
		symbolCache: map[string]EntitySymbol{},
		entity:      entity,
		tx:          tx,
	}
}

func (rs *rowCursorImpl) getSymbol(name string) EntitySymbol {
	result, found := rs.symbolCache[name]
	if !found {
		result = rs.entity.GetSymbol(name)
		if result != nil {
			rs.symbolCache[name] = result
		}
	}
	return result
}

func (rs *rowCursorImpl) GetSetSymbolTypes(name string) ast.SymbolTypes {
	return rs.entity.GetSetSymbolTypes(name)
}

func (rs *rowCursorImpl) NextRow(id []byte) {
	rs.currentRow = id
}

func (rs *rowCursorImpl) CurrentRow() []byte {
	return rs.currentRow
}

func (rs *rowCursorImpl) Tx() *bbolt.Tx {
	return rs.tx
}

func (rs *rowCursorImpl) GetSymbolType(name string) (ast.NodeType, bool) {
	symbol := rs.getSymbol(name)
	if symbol == nil {
		return 0, false
	}
	return symbol.GetType(), true
}

func (rs *rowCursorImpl) IsSet(name string) (bool, bool) {
	symbol := rs.getSymbol(name)
	if symbol == nil {
		return false, false
	}
	return symbol.IsSet(), true
}

func (rs *rowCursorImpl) OpenSetCursor(name string) ast.SetCursor {
	symbol := rs.getSymbol(name)
	if symbol == nil {
		pfxlog.Logger().Errorf("unknown symbol %v, should have been caught in symbol validation pass", name)
		return ast.NewEmptyCursor()
	}
	setRowSymbol, ok := symbol.(RuntimeEntitySetSymbol)
	if !ok {
		pfxlog.Logger().Errorf("attempting to iterate non-set symbol %v, should have been caught in symbol validation pass", name)
		return ast.NewEmptyCursor()
	}

	return setRowSymbol.OpenCursor(rs.tx, rs.currentRow)
}

func (rs *rowCursorImpl) OpenSetCursorForQuery(name string, query ast.Query) ast.SetCursor {
	symbol := rs.getSymbol(name)
	if symbol == nil {
		pfxlog.Logger().Errorf("unknown symbol %v, should have been caught in symbol validation pass", name)
		return ast.NewEmptyCursor()
	}
	setRowSymbol, ok := symbol.(RuntimeEntitySetSymbol)
	if !ok {
		pfxlog.Logger().Errorf("attempting to iterate non-set symbol %v, should have been caught in symbol validation pass", name)
		return ast.NewEmptyCursor()
	}
	setCursor := setRowSymbol.OpenCursor(rs.tx, rs.currentRow)
	return newCursorScanner(rs.tx, symbol.GetLinkedType(), setCursor, query)
}

func (rs *rowCursorImpl) EvalBool(name string) *bool {
	symbol := rs.getSymbol(name)
	if symbol == nil {
		pfxlog.Logger().Errorf("unknown symbol %v, should have been caught in symbol validation", name)
		return nil
	}
	return FieldToBool(symbol.Eval(rs.tx, rs.currentRow))
}

func (rs *rowCursorImpl) EvalString(name string) *string {
	symbol := rs.getSymbol(name)
	if symbol == nil {
		pfxlog.Logger().Errorf("unknown symbol %v, should have been caught in symbol validation", name)
		return nil
	}
	return FieldToString(symbol.Eval(rs.tx, rs.currentRow))
}

func (rs *rowCursorImpl) EvalInt64(name string) *int64 {
	symbol := rs.getSymbol(name)
	if symbol == nil {
		pfxlog.Logger().Errorf("unknown symbol %v, should have been caught in symbol validation", name)
		return nil
	}
	return FieldToInt64(symbol.Eval(rs.tx, rs.currentRow))
}

func (rs *rowCursorImpl) EvalFloat64(name string) *float64 {
	symbol := rs.getSymbol(name)
	if symbol == nil {
		pfxlog.Logger().Errorf("unknown symbol %v, should have been caught in symbol validation", name)
		return nil
	}
	return FieldToFloat64(symbol.Eval(rs.tx, rs.currentRow))
}

func (rs *rowCursorImpl) EvalDatetime(name string) *time.Time {
	symbol := rs.getSymbol(name)
	if symbol == nil {
		pfxlog.Logger().Errorf("unknown symbol %v, should have been caught in symbol validation", name)
		return nil
	}
	fieldType, val := symbol.Eval(rs.tx, rs.currentRow)
	return FieldToDatetime(fieldType, val, symbol.GetName())
}

func (rs *rowCursorImpl) IsNil(name string) bool {
	symbol := rs.getSymbol(name)
	if symbol == nil {
		pfxlog.Logger().Errorf("unknown symbol %v, should have been caught in symbol validation", name)
		return true
	}
	fieldType, _ := symbol.Eval(rs.tx, rs.currentRow)
	return fieldType == TypeNil
}
