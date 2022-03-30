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
)

type AllOfSetExprNode struct {
	name      string
	predicate BoolNode
}

func (node *AllOfSetExprNode) Symbol() string {
	return node.name
}

func (node *AllOfSetExprNode) String() string {
	return fmt.Sprintf("allOf(%v)", node.predicate)
}

func (node *AllOfSetExprNode) GetType() NodeType {
	return NodeTypeBool
}

func (node *AllOfSetExprNode) Accept(visitor Visitor) {
	visitor.VisitAllOfSetExprNodeStart(node)
	node.predicate.Accept(visitor)
	visitor.VisitAllOfSetExprNodeEnd(node)
}

func (node *AllOfSetExprNode) EvalBool(s Symbols) bool {
	cursor := s.OpenSetCursor(node.name)

	for cursor.IsValid() {
		if !node.predicate.EvalBool(s) {
			return false
		}
		cursor.Next()
	}
	return true
}

func (node *AllOfSetExprNode) IsConst() bool {
	return false
}

type AnyOfSetExprNode struct {
	name              string
	predicate         BoolNode
	seekablePredicate SeekOptimizableBoolNode
}

func (node *AnyOfSetExprNode) Symbol() string {
	return node.name
}

func (node *AnyOfSetExprNode) String() string {
	return fmt.Sprintf("anyOf(%v)", node.predicate)
}

func (node *AnyOfSetExprNode) GetType() NodeType {
	return NodeTypeBool
}

func (node *AnyOfSetExprNode) Accept(visitor Visitor) {
	visitor.VisitAnyOfSetExprNodeStart(node)
	node.predicate.Accept(visitor)
	visitor.VisitAnyOfSetExprNodeEnd(node)
}

func (node *AnyOfSetExprNode) EvalBool(s Symbols) bool {
	cursor := s.OpenSetCursor(node.name)

	if node.seekablePredicate != nil {
		if seekableCursor, ok := cursor.(TypeSeekableSetCursor); ok {
			return node.seekablePredicate.EvalBoolWithSeek(s, seekableCursor)
		}
	}

	for cursor.IsValid() {
		if node.predicate.EvalBool(s) {
			return true
		}
		cursor.Next()
	}
	return false
}

func (node *AnyOfSetExprNode) IsConst() bool {
	return false
}

type CountSetExprNode struct {
	symbol SymbolNode
	query  Query
}

func (node *CountSetExprNode) Symbol() string {
	return node.symbol.Symbol()
}

func (node *CountSetExprNode) String() string {
	return fmt.Sprintf("count(%v)", node.Symbol())
}

func (node *CountSetExprNode) GetType() NodeType {
	return NodeTypeInt64
}

func (node *CountSetExprNode) Accept(visitor Visitor) {
	visitor.VisitCountSetExprNodeStart(node)
	node.symbol.Accept(visitor)
	if node.query != nil {
		node.query.Accept(visitor)
	}
	visitor.VisitCountSetExprNodeEnd(node)
}

func (node *CountSetExprNode) EvalInt64(s Symbols) *int64 {
	var result int64
	var cursor SetCursor

	if node.query == nil {
		cursor = s.OpenSetCursor(node.Symbol())
	} else {
		cursor = s.OpenSetCursorForQuery(node.Symbol(), node.query)
	}

	for cursor.IsValid() {
		result++
		cursor.Next()
	}
	return &result
}

func (node *CountSetExprNode) EvalString(s Symbols) *string {
	result := node.EvalInt64(s)
	if result == nil {
		return nil
	}
	stringResult := strconv.FormatInt(*result, 10)
	return &stringResult
}

func (node *CountSetExprNode) ToFloat64() Float64Node {
	return &Int64ToFloat64Node{node}
}

func (node *CountSetExprNode) IsConst() bool {
	return false
}

type IsEmptySetExprNode struct {
	symbol SymbolNode
	query  Query
}

func (node *IsEmptySetExprNode) Symbol() string {
	return node.symbol.Symbol()
}

func (node *IsEmptySetExprNode) String() string {
	return fmt.Sprintf("isEmpty(%v)", node.Symbol())
}

func (node *IsEmptySetExprNode) GetType() NodeType {
	return NodeTypeBool
}

func (node *IsEmptySetExprNode) Accept(visitor Visitor) {
	visitor.VisitIsEmptySetExprNodeStart(node)
	node.symbol.Accept(visitor)
	if node.query != nil {
		node.query.Accept(visitor)
	}
	visitor.VisitIsEmptySetExprNodeEnd(node)
}

func (node *IsEmptySetExprNode) EvalBool(s Symbols) bool {
	var cursor SetCursor

	if node.query == nil {
		cursor = s.OpenSetCursor(node.Symbol())
	} else {
		cursor = s.OpenSetCursorForQuery(node.Symbol(), node.query)
	}
	return !cursor.IsValid()
}

func (node *IsEmptySetExprNode) IsConst() bool {
	return false
}
