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
	"fmt"
	"go.etcd.io/bbolt"
	"time"
)

type NodeType int

const (
	NodeTypeBool NodeType = iota
	NodeTypeDatetime
	NodeTypeFloat64
	NodeTypeInt64
	NodeTypeString
	NodeTypeAnyType

	NodeTypeOther
)

func NodeTypeName(nodeType NodeType) string {
	return nodeTypeNames[nodeType]
}

var nodeTypeNames = map[NodeType]string{
	NodeTypeString:   "string",
	NodeTypeInt64:    "number",
	NodeTypeFloat64:  "number",
	NodeTypeDatetime: "date",
	NodeTypeBool:     "bool",
	NodeTypeOther:    "other",
	NodeTypeAnyType:  "any",
}

type BinaryOp int

const (
	BinaryOpEQ BinaryOp = iota
	BinaryOpNEQ
	BinaryOpLT
	BinaryOpLTE
	BinaryOpGT
	BinaryOpGTE
	BinaryOpIn
	BinaryOpNotIn
	BinaryOpBetween
	BinaryOpNotBetween
	BinaryOpContains
	BinaryOpNotContains
	BinaryOpIContains
	BinaryOpNotIContains
)

func (op BinaryOp) IsCaseInsensitiveOp() bool {
	return op == BinaryOpIContains || op == BinaryOpNotIContains
}

func (op BinaryOp) String() string {
	return binaryOpNames[op]
}

var binaryOpNames = map[BinaryOp]string{
	BinaryOpEQ:           "=",
	BinaryOpNEQ:          "!=",
	BinaryOpLT:           "<",
	BinaryOpLTE:          "<=",
	BinaryOpGT:           ">",
	BinaryOpGTE:          ">=",
	BinaryOpIn:           "in",
	BinaryOpNotIn:        "not in",
	BinaryOpBetween:      "between",
	BinaryOpNotBetween:   "not between",
	BinaryOpContains:     "contains",
	BinaryOpNotContains:  "not icontains",
	BinaryOpIContains:    "contains",
	BinaryOpNotIContains: "not icontains",
}

var binaryOpValues = map[string]BinaryOp{
	"=":  BinaryOpEQ,
	"!=": BinaryOpNEQ,
	"<":  BinaryOpLT,
	"<=": BinaryOpLTE,
	">":  BinaryOpGT,
	">=": BinaryOpGTE,
}

type SetFunction int

const (
	SetFunctionAllOf SetFunction = iota
	SetFunctionAnyOf
	SetFunctionCount
	SetFunctionIsEmpty
)

var BoolNodeTrue = NewBoolConstNode(true)

var SetFunctionNames = map[SetFunction]string{
	SetFunctionAllOf:   "allOf",
	SetFunctionAnyOf:   "anyOf",
	SetFunctionCount:   "count",
	SetFunctionIsEmpty: "isEmpty",
}

type SymbolTypes interface {
	GetSymbolType(name string) (NodeType, bool)
	GetSetSymbolTypes(name string) SymbolTypes
	IsSet(name string) (bool, bool)
}

type Symbols interface {
	SymbolTypes
	EvalBool(name string) *bool
	EvalString(name string) *string
	EvalInt64(name string) *int64
	EvalFloat64(name string) *float64
	EvalDatetime(name string) *time.Time
	IsNil(name string) bool

	OpenSetCursor(name string) SetCursor
	OpenSetCursorForQuery(name string, query Query) SetCursor
}

type SetCursorProvider func(tx *bbolt.Tx, forward bool) SetCursor

type SetCursor interface {
	Next()
	IsValid() bool
	Current() []byte
}

type SeekableSetCursor interface {
	SetCursor
	Seek([]byte)
}

type TypeSeekableSetCursor interface {
	SeekableSetCursor
	SeekToString(val string)
}

type Node interface {
	fmt.Stringer
	GetType() NodeType
	Accept(visitor Visitor)
	IsConst() bool
}

type TypeTransformable interface {
	Node
	TypeTransform(s SymbolTypes) (Node, error)
}

type BoolNode interface {
	Node
	// NOTE: IF we want to reintroduce error reporting on Eval* at some point in the future, probably better plan
	// would be to incorporate into Symbols, which we can then retrieve at the top level, rather than threading
	// it through here everywhere, similar to how we do it with TypedBucket
	EvalBool(s Symbols) bool
}

type BoolTypeTransformable interface {
	BoolNode
	TypeTransformBool(s SymbolTypes) (BoolNode, error)
}

type DatetimeNode interface {
	Node
	EvalDatetime(s Symbols) *time.Time
}

type Float64Node interface {
	StringNode
	EvalFloat64(s Symbols) *float64
}

type Int64Node interface {
	StringNode
	EvalInt64(s Symbols) *int64
	ToFloat64() Float64Node
}

type StringNode interface {
	Node
	EvalString(s Symbols) *string
}

type SymbolNode interface {
	Node
	Symbol() string
}

type AsStringArrayable interface {
	AsStringArray() *StringArrayNode
}

type SortField interface {
	fmt.Stringer
	Symbol() string
	IsAscending() bool
}

type Query interface {
	BoolNode

	GetPredicate() BoolNode

	SetPredicate(BoolNode)

	// GetSortFields returns the fields on which to sort. Returning nil or empty means the default sort order
	// will be used, usually by id ascending
	GetSortFields() []SortField

	AdoptSortFields(query Query) error

	// GetSkip returns the number of rows to skip. nil, or a values less than one will mean no rows skipped
	GetSkip() *int64

	// GetLimit returns the maximum number of rows to return. Returning nil will use the system configured
	// default for max rows. Returning -1 means do not limit results.
	GetLimit() *int64

	SetSkip(int64)
	SetLimit(int64)
}
