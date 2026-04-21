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
	"strconv"
	"time"

	"github.com/openziti/foundation/v2/errorz"

	"github.com/pkg/errors"
)

var _ BoolNode = (*BoolSymbolNode)(nil)
var _ DatetimeNode = (*DatetimeSymbolNode)(nil)
var _ Int64Node = (*Int64SymbolNode)(nil)
var _ Float64Node = (*Float64SymbolNode)(nil)
var _ StringNode = (*StringSymbolNode)(nil)

var _ BoolNode = (*AnyTypeSymbolNode)(nil)
var _ DatetimeNode = (*AnyTypeSymbolNode)(nil)
var _ Int64Node = (*AnyTypeSymbolNode)(nil)
var _ Float64Node = (*AnyTypeSymbolNode)(nil)
var _ StringNode = (*AnyTypeSymbolNode)(nil)

func NewUntypedSymbolNode(symbol string) SymbolNode {
	return &UntypedSymbolNode{symbol: symbol}
}

type UntypedSymbolNode struct {
	symbol string
}

func (node *UntypedSymbolNode) Accept(visitor Visitor) {
	visitor.VisitSymbol(node.symbol, node.GetType())
	visitor.VisitUntypedSymbolNode(node)
}

func (node *UntypedSymbolNode) TypeTransform(s SymbolTypes) (Node, error) {
	kind, found := s.GetSymbolType(node.symbol)
	if !found {
		return nil, errors.Errorf("unknown symbol %v", node.symbol)
	}
	switch kind {
	case NodeTypeString:
		return &StringSymbolNode{symbol: node.symbol}, nil
	case NodeTypeBool:
		return &BoolSymbolNode{symbol: node.symbol}, nil
	case NodeTypeInt64:
		return &Int64SymbolNode{symbol: node.symbol}, nil
	case NodeTypeFloat64:
		return &Float64SymbolNode{symbol: node.symbol}, nil
	case NodeTypeDatetime:
		return &DatetimeSymbolNode{symbol: node.symbol}, nil
	case NodeTypeAnyType:
		return &AnyTypeSymbolNode{symbol: node.symbol}, nil
	}
	return nil, errors.Errorf("unhanded symbol type %v for symbol %v", kind, node.symbol)
}

func (node *UntypedSymbolNode) GetType() NodeType {
	return NodeTypeOther
}

func (node *UntypedSymbolNode) String() string {
	return node.symbol
}

func (node *UntypedSymbolNode) Symbol() string {
	return node.symbol
}

func (node *UntypedSymbolNode) IsConst() bool {
	return false
}

// BoolSymbolNode implements lookup of symbol values of type bool
type BoolSymbolNode struct {
	symbol string
}

func (node *BoolSymbolNode) Accept(visitor Visitor) {
	visitor.VisitSymbol(node.symbol, node.GetType())
	visitor.VisitBoolSymbolNode(node)
}

func (node *BoolSymbolNode) GetType() NodeType {
	return NodeTypeBool
}

func (node *BoolSymbolNode) EvalBool(s Symbols) bool {
	result := s.EvalBool(node.symbol)
	return result != nil && *result
}

func (node *BoolSymbolNode) String() string {
	return node.symbol
}

func (node *BoolSymbolNode) Symbol() string {
	return node.symbol
}

func (node *BoolSymbolNode) IsConst() bool {
	return false
}

// DatetimeSymbolNode implements lookup of symbol values of type datetime
type DatetimeSymbolNode struct {
	symbol string
}

func (node *DatetimeSymbolNode) Accept(visitor Visitor) {
	visitor.VisitSymbol(node.symbol, node.GetType())
	visitor.VisitDatetimeSymbolNode(node)
}

func (node *DatetimeSymbolNode) GetType() NodeType {
	return NodeTypeDatetime
}

func (node *DatetimeSymbolNode) EvalDatetime(s Symbols) *time.Time {
	return s.EvalDatetime(node.symbol)
}

func (node *DatetimeSymbolNode) String() string {
	return node.symbol
}

func (node *DatetimeSymbolNode) Symbol() string {
	return node.symbol
}

func (node *DatetimeSymbolNode) IsConst() bool {
	return false
}

// Float64SymbolNode implements lookup of symbol values of type float64
type Float64SymbolNode struct {
	symbol string
}

func (node *Float64SymbolNode) GetType() NodeType {
	return NodeTypeFloat64
}

func (node *Float64SymbolNode) Accept(visitor Visitor) {
	visitor.VisitSymbol(node.symbol, node.GetType())
	visitor.VisitFloat64SymbolNode(node)
}

func (node *Float64SymbolNode) EvalFloat64(s Symbols) *float64 {
	return s.EvalFloat64(node.symbol)
}

func (node *Float64SymbolNode) EvalString(s Symbols) *string {
	float64Val := s.EvalFloat64(node.symbol)
	if float64Val != nil {
		result := strconv.FormatFloat(*float64Val, 'f', -1, 64)
		return &result
	}
	return nil
}

func (node *Float64SymbolNode) String() string {
	return node.symbol
}

func (node *Float64SymbolNode) Symbol() string {
	return node.symbol
}

func (node *Float64SymbolNode) IsConst() bool {
	return false
}

// Int64SymbolNode implements lookup of symbol values of type int64
type Int64SymbolNode struct {
	symbol string
}

func (node *Int64SymbolNode) GetType() NodeType {
	return NodeTypeInt64
}

func (node *Int64SymbolNode) Accept(visitor Visitor) {
	visitor.VisitSymbol(node.symbol, node.GetType())
	visitor.VisitInt64SymbolNode(node)
}

func (node *Int64SymbolNode) EvalInt64(s Symbols) *int64 {
	return s.EvalInt64(node.symbol)
}

func (node *Int64SymbolNode) EvalString(s Symbols) *string {
	int64Val := s.EvalInt64(node.symbol)
	if int64Val != nil {
		result := strconv.FormatInt(*int64Val, 10)
		return &result
	}
	return nil
}

func (node *Int64SymbolNode) ToFloat64() Float64Node {
	return &Int64ToFloat64Node{node}
}

func (node *Int64SymbolNode) String() string {
	return node.symbol
}

func (node *Int64SymbolNode) Symbol() string {
	return node.symbol
}

func (node *Int64SymbolNode) IsConst() bool {
	return false
}

// StringSymbolNode implements lookup of symbol values of type string
type StringSymbolNode struct {
	symbol string
}

func (node *StringSymbolNode) GetType() NodeType {
	return NodeTypeString
}

func (node *StringSymbolNode) Accept(visitor Visitor) {
	visitor.VisitSymbol(node.symbol, node.GetType())
	visitor.VisitStringSymbolNode(node)
}

func (node *StringSymbolNode) EvalString(s Symbols) *string {
	return s.EvalString(node.symbol)
}

func (node *StringSymbolNode) String() string {
	return node.symbol
}

func (node *StringSymbolNode) Symbol() string {
	return node.symbol
}

func (node *StringSymbolNode) IsConst() bool {
	return false
}

// AnyTypeSymbolNode implements lookup of symbol values of any, meaning they can have any value type
type AnyTypeSymbolNode struct {
	symbol string
}

func (node *AnyTypeSymbolNode) GetType() NodeType {
	return NodeTypeAnyType
}

func (node *AnyTypeSymbolNode) Accept(visitor Visitor) {
	visitor.VisitSymbol(node.symbol, node.GetType())
	visitor.VisitAnyTypeSymbolNode(node)
}

func (node *AnyTypeSymbolNode) EvalBool(s Symbols) bool {
	result := s.EvalBool(node.symbol)
	return result != nil && *result
}

func (node *AnyTypeSymbolNode) EvalDatetime(s Symbols) *time.Time {
	return s.EvalDatetime(node.symbol)
}

func (node *AnyTypeSymbolNode) EvalInt64(s Symbols) *int64 {
	return s.EvalInt64(node.symbol)
}

func (node *AnyTypeSymbolNode) ToFloat64() Float64Node {
	return node
}

func (node *AnyTypeSymbolNode) EvalFloat64(s Symbols) *float64 {
	return s.EvalFloat64(node.symbol)
}

func (node *AnyTypeSymbolNode) EvalString(s Symbols) *string {
	return s.EvalString(node.symbol)
}

func (node *AnyTypeSymbolNode) String() string {
	return node.symbol
}

func (node *AnyTypeSymbolNode) Symbol() string {
	return node.symbol
}

func (node *AnyTypeSymbolNode) IsConst() bool {
	return false
}

type UnknownSymbolError struct {
	Symbol string
}

func (u UnknownSymbolError) Error() string {
	return fmt.Sprintf("unknown symbol '%v'", u.Symbol)
}

func NewUnknownSymbolError(symbol string) UnknownSymbolError {
	return UnknownSymbolError{Symbol: symbol}
}

type SymbolValidator struct {
	DefaultVisitor
	inSetFunction bool
	symbolTypes   SymbolTypes
	errorz.ErrorHolderImpl
	typeStack []SymbolTypes
	onDeck    SymbolTypes
}

func (visitor *SymbolValidator) VisitSetFunctionNodeStart(_ *SetFunctionNode) {
	visitor.inSetFunction = true
}

func (visitor *SymbolValidator) VisitSetFunctionNodeEnd(node *SetFunctionNode) {
	visitor.inSetFunction = false

	isSet, found := visitor.symbolTypes.IsSet(node.symbol.Symbol())
	if found && !isSet {
		visitor.SetError(errors.Errorf("symbol '%v' is not a set symbol but is used in set function %v",
			node.symbol.Symbol(), SetFunctionNames[node.setFunction]))
	}
}

func (visitor *SymbolValidator) VisitUntypedSymbolNode(node *UntypedSymbolNode) {
	isSet, found := visitor.symbolTypes.IsSet(node.Symbol())
	if !found {
		visitor.SetError(NewUnknownSymbolError(node.Symbol()))
		return
	}

	if !visitor.inSetFunction && isSet {
		visitor.SetError(errors.Errorf("symbol '%v' is a set symbol but is used in non-set function context", node.Symbol()))
	}

	if visitor.onDeck != nil {
		visitor.typeStack = append([]SymbolTypes{visitor.symbolTypes}, visitor.typeStack...)
		visitor.symbolTypes = visitor.onDeck
		visitor.onDeck = nil
	}
}

func (visitor *SymbolValidator) VisitUntypedSubQueryNodeStart(node *UntypedSubQueryNode) {
	symbolType := visitor.symbolTypes.GetSetSymbolTypes(node.Symbol())
	if symbolType == nil {
		visitor.SetError(errors.Errorf("attempt to subquery on non-entity symbol %v", node.Symbol()))
	} else {
		visitor.onDeck = symbolType
	}
}

func (visitor *SymbolValidator) VisitUntypedSubQueryNodeEnd(*UntypedSubQueryNode) {
	if !visitor.HasError() {
		visitor.symbolTypes = visitor.typeStack[0]
		visitor.typeStack = visitor.typeStack[1:]
	}
}
