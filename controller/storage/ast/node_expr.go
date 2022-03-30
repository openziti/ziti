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
	"github.com/michaelquigley/pfxlog"
	"github.com/pkg/errors"
	"strings"
)

// NotExprNode implements logical NOT on a wrapped boolean expression
type NotExprNode struct {
	expr BoolNode
}

func (node *NotExprNode) Accept(visitor Visitor) {
	visitor.VisitNotExprNodeStart(node)
	node.expr.Accept(visitor)
	visitor.VisitNotExprNodeEnd(node)
}

func (node *NotExprNode) String() string {
	return fmt.Sprintf("not (%v)", node.expr)
}

func (node *NotExprNode) GetType() NodeType {
	return NodeTypeBool
}

func (node *NotExprNode) EvalBool(s Symbols) bool {
	val := node.expr.EvalBool(s)
	return !val
}

func (node *NotExprNode) TypeTransformBool(s SymbolTypes) (BoolNode, error) {
	return node, transformBools(s, &node.expr)
}

func (node *NotExprNode) IsConst() bool {
	return node.expr.IsConst()
}

func NewAndExprNode(left, right BoolNode) *AndExprNode {
	return &AndExprNode{
		left:  left,
		right: right,
	}
}

// AndExprNode implements logical AND on two wrapped boolean expressions
type AndExprNode struct {
	left  BoolNode
	right BoolNode
}

func (node *AndExprNode) Accept(visitor Visitor) {
	visitor.VisitAndExprNodeStart(node)
	node.left.Accept(visitor)
	node.right.Accept(visitor)
	visitor.VisitAndExprNodeEnd(node)
}

func (node *AndExprNode) GetType() NodeType {
	return NodeTypeBool
}

func (node *AndExprNode) TypeTransformBool(s SymbolTypes) (BoolNode, error) {
	return node, transformBools(s, &node.left, &node.right)
}

func (node *AndExprNode) EvalBool(s Symbols) bool {
	if !node.left.EvalBool(s) {
		return false
	}
	return node.right.EvalBool(s)
}

func (node *AndExprNode) String() string {
	return fmt.Sprintf("%v && %v", node.left, node.right)
}

func (node *AndExprNode) IsConst() bool {
	return false
}

// OrExprNode implements logical OR on two wrapped boolean expressions
type OrExprNode struct {
	left  BoolNode
	right BoolNode
}

func (node *OrExprNode) Accept(visitor Visitor) {
	visitor.VisitOrExprNodeStart(node)
	node.left.Accept(visitor)
	node.right.Accept(visitor)
	visitor.VisitOrExprNodeEnd(node)
}

func (node *OrExprNode) GetType() NodeType {
	return NodeTypeBool
}

func (node *OrExprNode) TypeTransformBool(s SymbolTypes) (BoolNode, error) {
	return node, transformBools(s, &node.left, &node.right)
}

func (node *OrExprNode) EvalBool(s Symbols) bool {
	leftResult := node.left.EvalBool(s)
	if leftResult {
		return true
	}
	return node.right.EvalBool(s)
}

func (node *OrExprNode) String() string {
	return fmt.Sprintf("%v || %v", node.left, node.right)
}

func (node *OrExprNode) IsConst() bool {
	return false
}

type SeekOptimizableBoolNode interface {
	IsSeekable() bool
	EvalBoolWithSeek(s Symbols, cursor TypeSeekableSetCursor) bool
}

type BinaryBoolExprNode struct {
	left  BoolNode
	right BoolNode
	op    BinaryOp
}

func (node *BinaryBoolExprNode) Accept(visitor Visitor) {
	visitor.VisitBinaryBoolExprNodeStart(node)
	node.left.Accept(visitor)
	node.right.Accept(visitor)
	visitor.VisitBinaryBoolExprNodeEnd(node)
}

func (*BinaryBoolExprNode) GetType() NodeType {
	return NodeTypeBool
}

func (node *BinaryBoolExprNode) EvalBool(s Symbols) bool {
	leftResult := node.left.EvalBool(s)
	rightResult := node.right.EvalBool(s)

	switch node.op {
	case BinaryOpEQ:
		return leftResult == rightResult
	case BinaryOpNEQ:
		return leftResult != rightResult
	}

	pfxlog.Logger().Errorf("unhandled boolean binary expression type %v", node.op)
	return false
}

func (node *BinaryBoolExprNode) String() string {
	return fmt.Sprintf("%v %v %v", node.left, binaryOpNames[node.op], node.right)
}

func (node *BinaryBoolExprNode) IsConst() bool {
	return false
}

type BinaryDatetimeExprNode struct {
	left  DatetimeNode
	right DatetimeNode
	op    BinaryOp
}

func (node *BinaryDatetimeExprNode) Accept(visitor Visitor) {
	visitor.VisitBinaryDatetimeExprNodeStart(node)
	node.left.Accept(visitor)
	node.right.Accept(visitor)
	visitor.VisitBinaryDatetimeExprNodeEnd(node)
}

func (*BinaryDatetimeExprNode) GetType() NodeType {
	return NodeTypeBool
}

func (node *BinaryDatetimeExprNode) EvalBool(s Symbols) bool {
	leftResult := node.left.EvalDatetime(s)
	rightResult := node.right.EvalDatetime(s)

	if leftResult == nil || rightResult == nil {
		return false
	}

	switch node.op {
	case BinaryOpEQ:
		return leftResult.Equal(*rightResult)
	case BinaryOpNEQ:
		return !leftResult.Equal(*rightResult)
	case BinaryOpLT:
		return leftResult.Before(*rightResult)
	case BinaryOpLTE:
		return !leftResult.After(*rightResult)
	case BinaryOpGT:
		return leftResult.After(*rightResult)
	case BinaryOpGTE:
		return !leftResult.Before(*rightResult)
	}

	pfxlog.Logger().Errorf("unhandled datetime binary expression type %v", node.op)
	return false
}

func (node *BinaryDatetimeExprNode) String() string {
	return fmt.Sprintf("%v %v %v", node.left, binaryOpNames[node.op], node.right)
}

func (node *BinaryDatetimeExprNode) IsConst() bool {
	return false
}

type BinaryFloat64ExprNode struct {
	left  Float64Node
	right Float64Node
	op    BinaryOp
}

func (node *BinaryFloat64ExprNode) Accept(visitor Visitor) {
	visitor.VisitBinaryFloat64ExprNodeStart(node)
	node.left.Accept(visitor)
	node.right.Accept(visitor)
	visitor.VisitBinaryFloat64ExprNodeEnd(node)
}

func (node *BinaryFloat64ExprNode) GetType() NodeType {
	return NodeTypeFloat64
}

func (node *BinaryFloat64ExprNode) EvalBool(s Symbols) bool {
	leftResult := node.left.EvalFloat64(s)
	rightResult := node.right.EvalFloat64(s)

	if leftResult == nil || rightResult == nil {
		return false
	}

	switch node.op {
	case BinaryOpEQ:
		return *leftResult == *rightResult
	case BinaryOpNEQ:
		return *leftResult != *rightResult
	case BinaryOpLT:
		return *leftResult < *rightResult
	case BinaryOpLTE:
		return *leftResult <= *rightResult
	case BinaryOpGT:
		return *leftResult > *rightResult
	case BinaryOpGTE:
		return *leftResult >= *rightResult
	}

	pfxlog.Logger().Errorf("unhandled float64 binary expression type %v", node.op)
	return false
}

func (node *BinaryFloat64ExprNode) String() string {
	return fmt.Sprintf("%v %v %v", node.left, binaryOpNames[node.op], node.right)
}

func (node *BinaryFloat64ExprNode) IsConst() bool {
	return false
}

type BinaryInt64ExprNode struct {
	left  Int64Node
	right Int64Node
	op    BinaryOp
}

func (node *BinaryInt64ExprNode) Accept(visitor Visitor) {
	visitor.VisitBinaryInt64ExprNodeStart(node)
	node.left.Accept(visitor)
	node.right.Accept(visitor)
	visitor.VisitBinaryInt64ExprNodeEnd(node)
}

func (node *BinaryInt64ExprNode) GetType() NodeType {
	return NodeTypeBool
}

func (node *BinaryInt64ExprNode) EvalBool(s Symbols) bool {
	leftResult := node.left.EvalInt64(s)
	rightResult := node.right.EvalInt64(s)

	if leftResult == nil || rightResult == nil {
		return false
	}

	switch node.op {
	case BinaryOpEQ:
		return *leftResult == *rightResult
	case BinaryOpNEQ:
		return *leftResult != *rightResult
	case BinaryOpLT:
		return *leftResult < *rightResult
	case BinaryOpLTE:
		return *leftResult <= *rightResult
	case BinaryOpGT:
		return *leftResult > *rightResult
	case BinaryOpGTE:
		return *leftResult >= *rightResult
	}

	pfxlog.Logger().Errorf("unhandled int64 binary expression type %v", node.op)
	return false
}

func (node *BinaryInt64ExprNode) String() string {
	return fmt.Sprintf("%v %v %v", node.left, binaryOpNames[node.op], node.right)
}

func (node *BinaryInt64ExprNode) IsConst() bool {
	return false
}

type BinaryStringExprNode struct {
	left  StringNode
	right StringNode
	op    BinaryOp
}

func (node *BinaryStringExprNode) Accept(visitor Visitor) {
	visitor.VisitBinaryStringExprNodeStart(node)
	node.left.Accept(visitor)
	node.right.Accept(visitor)
	visitor.VisitBinaryStringExprNodeEnd(node)
}

func (*BinaryStringExprNode) GetType() NodeType {
	return NodeTypeBool
}

func (node *BinaryStringExprNode) IsSeekable() bool {
	return (node.op == BinaryOpEQ || node.op == BinaryOpNEQ) &&
		(node.left.IsConst() || node.right.IsConst())
}

func (node *BinaryStringExprNode) EvalBoolWithSeek(s Symbols, cursor TypeSeekableSetCursor) bool {
	if rightResult := node.right.EvalString(s); rightResult != nil {
		cursor.SeekToString(*rightResult)
		if cursor.IsValid() {
			return node.EvalBool(s)
		}
	}
	return false
}

func (node *BinaryStringExprNode) EvalBool(s Symbols) bool {
	leftResult := node.left.EvalString(s)
	rightResult := node.right.EvalString(s)

	if leftResult == nil || rightResult == nil {
		return false
	}

	switch node.op {
	case BinaryOpEQ:
		return *leftResult == *rightResult
	case BinaryOpNEQ:
		return *leftResult != *rightResult
	case BinaryOpLT:
		return *leftResult < *rightResult
	case BinaryOpLTE:
		return *leftResult <= *rightResult
	case BinaryOpGT:
		return *leftResult > *rightResult
	case BinaryOpGTE:
		return *leftResult >= *rightResult
	case BinaryOpContains:
		return strings.Contains(*leftResult, *rightResult)
	case BinaryOpNotContains:
		return !strings.Contains(*leftResult, *rightResult)
	}

	pfxlog.Logger().Errorf("unhandled string binary expression type %v", node.op)
	return false
}

func (node *BinaryStringExprNode) String() string {
	return fmt.Sprintf("%v %v %v", node.left, binaryOpNames[node.op], node.right)
}

func (node *BinaryStringExprNode) IsConst() bool {
	return false
}

type IsNilExprNode struct {
	symbol SymbolNode
	op     BinaryOp
}

func (node *IsNilExprNode) Accept(visitor Visitor) {
	visitor.VisitIsNilExprNodeStart(node)
	node.symbol.Accept(visitor)
	visitor.VisitIsNilExprNodeEnd(node)
}

func (*IsNilExprNode) GetType() NodeType {
	return NodeTypeBool
}

func (node *IsNilExprNode) EvalBool(s Symbols) bool {
	isNil := s.IsNil(node.symbol.Symbol())

	switch node.op {
	case BinaryOpEQ:
		return isNil
	case BinaryOpNEQ:
		return !isNil
	}

	pfxlog.Logger().Errorf("unhandled binary expression type %v", node)
	return true
}

func (node *IsNilExprNode) String() string {
	return fmt.Sprintf("%v %v null", node.symbol, binaryOpNames[node.op])
}

func (node *IsNilExprNode) IsConst() bool {
	return false
}

func NewInt64BetweenOp(nodes []Int64Node) (*Int64BetweenExprNode, error) {
	if len(nodes) != 3 {
		return nil, errors.Errorf("incorrect number of values provided to Int64BetweenExprNode: %v", len(nodes))
	}
	return &Int64BetweenExprNode{
		left:  nodes[0],
		lower: nodes[1],
		upper: nodes[2],
	}, nil
}

type Int64BetweenExprNode struct {
	left  Int64Node
	lower Int64Node
	upper Int64Node
}

func (node *Int64BetweenExprNode) Accept(visitor Visitor) {
	visitor.VisitInt64BetweenExprNodeStart(node)
	node.left.Accept(visitor)
	node.lower.Accept(visitor)
	node.upper.Accept(visitor)
	visitor.VisitInt64BetweenExprNodeEnd(node)
}

func (*Int64BetweenExprNode) GetType() NodeType {
	return NodeTypeBool
}

func (node *Int64BetweenExprNode) EvalBool(s Symbols) bool {
	leftResult := node.left.EvalInt64(s)

	if leftResult == nil {
		return false
	}

	lowerResult := node.lower.EvalInt64(s)
	if lowerResult == nil {
		return false
	}

	upperResult := node.upper.EvalInt64(s)
	if upperResult == nil {
		return false
	}

	return *leftResult >= *lowerResult && *leftResult < *upperResult
}

func (node *Int64BetweenExprNode) String() string {
	return fmt.Sprintf("%v between %v and %v", node.left, node.lower, node.upper)
}

func (node *Int64BetweenExprNode) IsConst() bool {
	return false
}

func NewFloat64BetweenOp(nodes []Float64Node) (*Float64BetweenExprNode, error) {
	if len(nodes) != 3 {
		return nil, errors.Errorf("incorrect number of values provided to Float64BetweenExprNode: %v", len(nodes))
	}
	return &Float64BetweenExprNode{
		left:  nodes[0],
		lower: nodes[1],
		upper: nodes[2],
	}, nil
}

type Float64BetweenExprNode struct {
	left  Float64Node
	lower Float64Node
	upper Float64Node
}

func (node *Float64BetweenExprNode) Accept(visitor Visitor) {
	visitor.VisitFloat64BetweenExprNodeStart(node)
	node.left.Accept(visitor)
	node.lower.Accept(visitor)
	node.upper.Accept(visitor)
	visitor.VisitFloat64BetweenExprNodeEnd(node)
}

func (*Float64BetweenExprNode) GetType() NodeType {
	return NodeTypeBool
}

func (node *Float64BetweenExprNode) EvalBool(s Symbols) bool {
	leftResult := node.left.EvalFloat64(s)
	if leftResult == nil {
		return false
	}

	lowerResult := node.lower.EvalFloat64(s)
	if lowerResult == nil {
		return false
	}

	upperResult := node.upper.EvalFloat64(s)
	if upperResult == nil {
		return false
	}

	return *leftResult >= *lowerResult && *leftResult < *upperResult
}

func (node *Float64BetweenExprNode) String() string {
	return fmt.Sprintf("%v between %v and %v", node.left, node.lower, node.upper)
}

func (node *Float64BetweenExprNode) IsConst() bool {
	return false
}

type DatetimeBetweenExprNode struct {
	left  DatetimeNode
	lower DatetimeNode
	upper DatetimeNode
}

func (node *DatetimeBetweenExprNode) Accept(visitor Visitor) {
	visitor.VisitDatetimeBetweenExprNodeStart(node)
	node.left.Accept(visitor)
	node.lower.Accept(visitor)
	node.upper.Accept(visitor)
	visitor.VisitDatetimeBetweenExprNodeEnd(node)
}

func (*DatetimeBetweenExprNode) GetType() NodeType {
	return NodeTypeBool
}

func (node *DatetimeBetweenExprNode) EvalBool(s Symbols) bool {
	leftResult := node.left.EvalDatetime(s)

	if leftResult == nil {
		return false
	}

	lowerResult := node.lower.EvalDatetime(s)
	if lowerResult == nil {
		return false
	}

	upperResult := node.upper.EvalDatetime(s)
	if upperResult == nil {
		return false
	}

	return (leftResult.Equal(*lowerResult) || leftResult.After(*lowerResult)) && leftResult.Before(*upperResult)
}

func (node *DatetimeBetweenExprNode) String() string {
	return fmt.Sprintf("%v between %v and %v", node.left, node.lower, node.upper)
}

func (node *DatetimeBetweenExprNode) IsConst() bool {
	return false
}
