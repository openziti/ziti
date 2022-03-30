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
	"github.com/openziti/storage/ast"
	"github.com/openziti/foundation/util/errorz"
	"go.etcd.io/bbolt"
)

const (
	SortMax = 5
)

type RowComparator interface {
	Compare(rowId1, rowId2 RowCursor) int
}

type Scanner interface {
	Scan(tx *bbolt.Tx, query ast.Query) ([]string, int64, error)
	ScanCursor(tx *bbolt.Tx, cursorProvider ast.SetCursorProvider, query ast.Query) ([]string, int64, error)
}

type RowCursor interface {
	CurrentRow() []byte
	Tx() *bbolt.Tx
}

type EntitySymbol interface {
	GetStore() ListStore
	GetLinkedType() ListStore
	GetPath() []string
	GetType() ast.NodeType
	GetName() string
	IsSet() bool
	Eval(tx *bbolt.Tx, rowId []byte) (FieldType, []byte)
}

type SymbolMapper interface {
	Map(source EntitySymbol, fieldType FieldType, value []byte) (FieldType, []byte)
}

type symbolMapWrapper struct {
	EntitySymbol
	SymbolMapper
}

func (wrapper *symbolMapWrapper) Eval(tx *bbolt.Tx, rowId []byte) (FieldType, []byte) {
	fieldType, value := wrapper.EntitySymbol.Eval(tx, rowId)
	return wrapper.Map(wrapper.EntitySymbol, fieldType, value)
}

type NotNilStringMapper struct {
}

func (n NotNilStringMapper) Map(_ EntitySymbol, fieldType FieldType, value []byte) (FieldType, []byte) {
	if fieldType == TypeNil {
		return TypeString, []byte("")
	}
	return fieldType, value
}

type MapContext struct {
	fieldType FieldType
	val       []byte
	newType   FieldType
	newVal    []byte
	replace   bool
	stop      bool
	errorz.ErrorHolderImpl
}

func (ctx *MapContext) next(fieldType FieldType, val []byte) {
	ctx.fieldType = fieldType
	ctx.val = val
	ctx.newVal = nil
	ctx.replace = false
}

func (ctx *MapContext) Type() FieldType {
	return ctx.fieldType
}

func (ctx *MapContext) Value() []byte {
	return ctx.val
}

func (ctx *MapContext) ValueS() string {
	return string(ctx.val)
}

func (ctx *MapContext) Delete() {
	ctx.replace = true
}

func (ctx *MapContext) Replace(fieldType FieldType, val []byte) {
	ctx.fieldType = fieldType
	ctx.newVal = val
	ctx.replace = true
}

func (ctx *MapContext) ReplaceS(val string) {
	ctx.Replace(TypeString, []byte(val))
}

func (ctx *MapContext) Stop() {
	ctx.stop = true
}

type EntitySetSymbol interface {
	EntitySymbol
	GetRuntimeSymbol() RuntimeEntitySetSymbol
	EvalStringList(tx *bbolt.Tx, key []byte) []string
	Map(tx *bbolt.Tx, key []byte, f func(ctx *MapContext)) error
}

type RuntimeEntitySetSymbol interface {
	EntitySymbol
	OpenCursor(tx *bbolt.Tx, rowId []byte) ast.SetCursor
}
