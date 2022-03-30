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
	"reflect"

	"github.com/pkg/errors"
)

func transformTypes(s SymbolTypes, nodes ...*Node) error {
	for _, node := range nodes {
		if sp, ok := (*node).(TypeTransformable); ok {
			transformed, err := sp.TypeTransform(s)
			if err != nil {
				return err
			}
			*node = transformed
		}

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

func toInt64Nodes(nodes ...Node) ([]Int64Node, bool) {
	var result []Int64Node
	for _, node := range nodes {
		if int64Node, ok := node.(Int64Node); ok {
			result = append(result, int64Node)
		} else {
			return nil, false
		}
	}
	return result, true
}

func toFloat64Nodes(nodes ...Node) ([]Float64Node, bool) {
	var result []Float64Node
	for _, node := range nodes {
		if int64Node, ok := node.(Int64Node); ok {
			result = append(result, int64Node.ToFloat64())
		} else if float64Node, ok := node.(Float64Node); ok {
			result = append(result, float64Node)
		} else {
			return nil, false
		}
	}
	return result, true
}

type boolBinaryOp int

const (
	AndOp boolBinaryOp = iota
	OrOp
)

type BooleanLogicExprNode struct {
	left  Node
	right Node
	op    boolBinaryOp
}

func (node *BooleanLogicExprNode) Accept(visitor Visitor) {
	visitor.VisitBooleanLogicExprNodeStart(node)
	node.left.Accept(visitor)
	node.right.Accept(visitor)
	visitor.VisitBooleanLogicExprNodeEnd(node)
}

func (node *BooleanLogicExprNode) GetType() NodeType {
	return NodeTypeBool
}

func (node *BooleanLogicExprNode) EvalBool(_ Symbols) bool {
	pfxlog.Logger().Errorf("cannot evaluate transitory binary bool op node %v", node)
	return false
}

func (node *BooleanLogicExprNode) String() string {
	opName := "AND"
	if node.op == OrOp {
		opName = "OR"
	}
	return fmt.Sprintf("%v %v %v", node.left, opName, node.right)
}

func (node *BooleanLogicExprNode) TypeTransformBool(s SymbolTypes) (BoolNode, error) {
	if err := transformTypes(s, &node.left, &node.right); err != nil {
		return node, err
	}
	left, ok := node.left.(BoolNode)
	if !ok {
		return node, errors.Errorf("boolean logic expression LHS is of type %v, not bool", reflect.TypeOf(node.left))
	}

	right, ok := node.right.(BoolNode)
	if !ok {
		return node, errors.Errorf("boolean logic expression RHS is of type %v, not bool", reflect.TypeOf(node.right))
	}

	if node.op == AndOp {
		return &AndExprNode{left, right}, nil
	}
	if node.op == OrOp {
		return &OrExprNode{left, right}, nil
	}
	return node, errors.Errorf("unsupported boolean logic expression operation %v", node.op)
}

func (node *BooleanLogicExprNode) IsConst() bool {
	return false
}

type BinaryExprNode struct {
	left  Node
	right Node
	op    BinaryOp
}

func (node *BinaryExprNode) Accept(visitor Visitor) {
	visitor.VisitBinaryExprNodeStart(node)
	node.left.Accept(visitor)
	node.right.Accept(visitor)
	visitor.VisitBinaryExprNodeEnd(node)
}

func (node *BinaryExprNode) GetType() NodeType {
	return NodeTypeBool
}

func (node *BinaryExprNode) EvalBool(_ Symbols) bool {
	pfxlog.Logger().Errorf("cannot evaluate transitory binary op node %v", node)
	return false
}

func (node *BinaryExprNode) TypeTransformBool(s SymbolTypes) (BoolNode, error) {
	if err := transformTypes(s, &node.left, &node.right); err != nil {
		return node, err
	}

	setFunction, isSetFunction := node.left.(*SetFunctionNode)
	if isSetFunction && setFunction.IsCompare() {
		node.left = setFunction.symbol
	}

	typedExpr, err := node.getTypedExpr()
	if err != nil {
		return nil, err
	}

	if isSetFunction && setFunction.IsCompare() {
		return setFunction.MoveUpTree(typedExpr)
	}

	return typedExpr, nil
}

func (node *BinaryExprNode) getTypedExpr() (BoolNode, error) {
	if _, isNull := node.right.(NullConstNode); isNull {
		return node.handleIsNullOps()
	}

	if node.op == BinaryOpContains || node.op == BinaryOpNotContains {
		return node.handleStringOps()
	}

	// use symbol type to drive operation type selection, unless type is any
	nodeType := node.left.GetType()
	if nodeType == NodeTypeAnyType {
		nodeType = node.right.GetType()
	}

	if nodeType == NodeTypeBool {
		return node.handleBoolOps()
	}
	if nodeType == NodeTypeDatetime {
		return node.handleDatetimeOps()
	}
	if nodeType == NodeTypeFloat64 {
		return node.handleFloat64Ops()
	}
	if nodeType == NodeTypeInt64 {
		return node.handleInt64Ops()
	}
	if nodeType == NodeTypeString {
		return node.handleStringOps()
	}

	return node.invalidOpTypes()
}

func (node *BinaryExprNode) handleIsNullOps() (BoolNode, error) {
	symbolNode, isSymbol := node.left.(SymbolNode)
	if isSymbol && (node.op == BinaryOpEQ || node.op == BinaryOpNEQ) {
		return &IsNilExprNode{
			symbol: symbolNode,
			op:     node.op,
		}, nil
	}
	return node.invalidOpTypes()
}

func (node *BinaryExprNode) handleBoolOps() (BoolNode, error) {
	if node.right.GetType() == NodeTypeBool && (node.op == BinaryOpEQ || node.op == BinaryOpNEQ) {
		return &BinaryBoolExprNode{
			left:  node.left.(BoolNode),
			right: node.right.(BoolNode),
			op:    node.op,
		}, nil
	}
	return node.invalidOpTypes()
}

func (node *BinaryExprNode) handleStringOps() (BoolNode, error) {
	left, ok := node.left.(StringNode)
	if ok {
		right, ok := node.right.(StringNode)
		if ok {
			return &BinaryStringExprNode{
				left:  left,
				right: right,
				op:    node.op,
			}, nil
		}
	}
	return node.invalidOpTypes()
}

func (node *BinaryExprNode) handleInt64Ops() (BoolNode, error) {
	left := node.left.(Int64Node)
	if node.right.GetType() == NodeTypeInt64 {
		return &BinaryInt64ExprNode{
			left:  left,
			right: node.right.(Int64Node),
			op:    node.op,
		}, nil
	}
	if node.right.GetType() == NodeTypeFloat64 {
		return &BinaryFloat64ExprNode{
			left:  left.ToFloat64(),
			right: node.right.(Float64Node),
			op:    node.op,
		}, nil
	}
	return node.invalidOpTypes()
}

func (node *BinaryExprNode) handleFloat64Ops() (BoolNode, error) {
	left := node.left.(Float64Node)
	if node.right.GetType() == NodeTypeFloat64 {
		return &BinaryFloat64ExprNode{
			left:  left,
			right: node.right.(Float64Node),
			op:    node.op,
		}, nil
	}

	if node.right.GetType() == NodeTypeInt64 {
		right := node.right.(Int64Node)
		return &BinaryFloat64ExprNode{
			left:  left,
			right: right.ToFloat64(),
			op:    node.op,
		}, nil
	}
	return node.invalidOpTypes()
}

func (node *BinaryExprNode) handleDatetimeOps() (BoolNode, error) {
	left := node.left.(DatetimeNode)
	if node.right.GetType() == NodeTypeDatetime {
		return &BinaryDatetimeExprNode{
			left:  left,
			right: node.right.(DatetimeNode),
			op:    node.op,
		}, nil
	}
	return node.invalidOpTypes()
}

func (node *BinaryExprNode) invalidOpTypes() (BoolNode, error) {
	return node, errors.Errorf("operation %v is not supported with operands types %v, %v",
		node, nodeTypeNames[node.left.GetType()], nodeTypeNames[node.right.GetType()])
}

func (node *BinaryExprNode) String() string {
	return fmt.Sprintf("%v %v %v", node.left, binaryOpNames[node.op], node.right)
}

func (node *BinaryExprNode) IsConst() bool {
	return false
}

type Int64ToFloat64Node struct {
	wrapped Int64Node
}

func (node *Int64ToFloat64Node) GetType() NodeType {
	return NodeTypeFloat64
}

func (node *Int64ToFloat64Node) Accept(visitor Visitor) {
	visitor.VisitInt64ToFloat64NodeStart(node)
	node.wrapped.Accept(visitor)
	visitor.VisitInt64ToFloat64NodeEnd(node)
}

func (node *Int64ToFloat64Node) EvalFloat64(s Symbols) *float64 {
	result := node.wrapped.EvalInt64(s)
	if result == nil {
		return nil
	}
	floatResult := float64(*result)
	return &floatResult
}

func (node *Int64ToFloat64Node) EvalString(s Symbols) *string {
	return node.wrapped.EvalString(s)
}

func (node *Int64ToFloat64Node) String() string {
	return fmt.Sprintf("float64(%v)", node.wrapped)
}

func (node *Int64ToFloat64Node) IsConst() bool {
	return node.wrapped.IsConst()
}

func NewInArrayExprNode(left, right Node) *InArrayExprNode {
	return &InArrayExprNode{
		left:  left,
		right: right,
	}
}

// InArrayExprNode is transitory node that handles conversion from untyped to typed IN nodes
type InArrayExprNode struct {
	left  Node
	right Node
}

func (node *InArrayExprNode) Accept(visitor Visitor) {
	visitor.VisitInArrayExprNodeStart(node)
	node.left.Accept(visitor)
	node.right.Accept(visitor)
	visitor.VisitInArrayExprNodeEnd(node)
}

func (node *InArrayExprNode) String() string {
	return fmt.Sprintf("%v in %v", node.left, node.right)
}

func (*InArrayExprNode) GetType() NodeType {
	return NodeTypeBool
}

func (node *InArrayExprNode) EvalBool(_ Symbols) bool {
	pfxlog.Logger().Errorf("cannot evaluate transitory in node %v", node)
	return false
}

func (node *InArrayExprNode) TypeTransformBool(s SymbolTypes) (BoolNode, error) {
	if err := transformTypes(s, &node.left, &node.right); err != nil {
		return node, err
	}

	setFunction, isSetFunction := node.left.(*SetFunctionNode)
	if isSetFunction {
		node.left = setFunction.symbol
	}

	typedExpr, err := node.getTypedExpr()
	if err != nil {
		return nil, err
	}

	if isSetFunction {
		return setFunction.MoveUpTree(typedExpr)
	}

	return typedExpr, nil
}

func (node *InArrayExprNode) getTypedExpr() (BoolNode, error) {
	if leftDatetime, ok := node.left.(DatetimeNode); ok {
		if rightDatetimeArray, ok := node.right.(*DatetimeArrayNode); ok {
			return &InDatetimeArrayExprNode{left: leftDatetime, right: rightDatetimeArray}, nil
		}
	}

	if leftInt, ok := node.left.(Int64Node); ok {
		if rightIntArr, ok := node.right.(*Int64ArrayNode); ok {
			return &InInt64ArrayExprNode{left: leftInt, right: rightIntArr}, nil
		}
		leftFloat := leftInt.ToFloat64()
		if rightFloatArr, ok := node.right.(*Float64ArrayNode); ok {
			return &InFloat64ArrayExprNode{left: leftFloat, right: rightFloatArr}, nil
		}
	}

	if leftFloat, ok := node.left.(Float64Node); ok {
		if rightIntArr, ok := node.right.(*Int64ArrayNode); ok {
			return &InFloat64ArrayExprNode{left: leftFloat, right: rightIntArr.ToFloat64ArrayNode()}, nil
		}
		if rightFloatArr, ok := node.right.(*Float64ArrayNode); ok {
			return &InFloat64ArrayExprNode{left: leftFloat, right: rightFloatArr}, nil
		}
	}

	if leftStr, ok := node.left.(StringNode); ok {
		if rightStrArray, ok := node.right.(AsStringArrayable); ok {
			return &InStringArrayExprNode{left: leftStr, right: rightStrArray.AsStringArray()}, nil
		}
	}

	return node, errors.Errorf("operation %v is not supported with operands types %v, %v",
		node, nodeTypeNames[node.left.GetType()], nodeTypeNames[node.right.GetType()])
}

func (node *InArrayExprNode) IsConst() bool {
	return false
}

// BetweenExprNode is transitory node that handles conversion from untyped to typed BETWEEN nodes
type BetweenExprNode struct {
	left  Node
	lower Node
	upper Node
}

func (node *BetweenExprNode) Accept(visitor Visitor) {
	visitor.VisitBetweenExprNodeStart(node)
	node.left.Accept(visitor)
	node.lower.Accept(visitor)
	node.upper.Accept(visitor)
	visitor.VisitBetweenExprNodeEnd(node)
}

func (node *BetweenExprNode) String() string {
	return fmt.Sprintf("%v between %v and %v", node.left, node.lower, node.upper)
}

func (*BetweenExprNode) GetType() NodeType {
	return NodeTypeBool
}

func (node *BetweenExprNode) EvalBool(_ Symbols) bool {
	pfxlog.Logger().Errorf("cannot evaluate transitory between node %v", node)
	return false
}

func (node *BetweenExprNode) TypeTransformBool(s SymbolTypes) (BoolNode, error) {
	if err := transformTypes(s, &node.left, &node.lower, &node.upper); err != nil {
		return node, err
	}

	setFunction, isSetFunction := node.left.(*SetFunctionNode)
	if isSetFunction {
		node.left = setFunction.symbol
	}

	typedExpr, err := node.getTypedExpr()
	if err != nil {
		return nil, err
	}

	if isSetFunction {
		return setFunction.MoveUpTree(typedExpr)
	}

	return typedExpr, nil
}

func (node *BetweenExprNode) getTypedExpr() (BoolNode, error) {
	if leftDatetime, ok := node.left.(DatetimeNode); ok {
		lowerDatetime := node.lower.(DatetimeNode)
		upperDatetime := node.upper.(DatetimeNode)
		return &DatetimeBetweenExprNode{left: leftDatetime, lower: lowerDatetime, upper: upperDatetime}, nil
	}

	if int64Nodes, ok := toInt64Nodes(node.left, node.lower, node.upper); ok {
		return NewInt64BetweenOp(int64Nodes)
	}

	if float64Nodes, ok := toFloat64Nodes(node.left, node.lower, node.upper); ok {
		return NewFloat64BetweenOp(float64Nodes)
	}

	return node, errors.Errorf("operation between is not supported with operands types %v, %v, %v",
		reflect.TypeOf(node.left), reflect.TypeOf(node.lower), reflect.TypeOf(node.lower))
}

func (node *BetweenExprNode) IsConst() bool {
	return false
}

type SetFunctionNode struct {
	setFunction SetFunction
	symbol      SymbolNode
}

func (node *SetFunctionNode) Accept(visitor Visitor) {
	visitor.VisitSetFunctionNodeStart(node)
	node.symbol.Accept(visitor)
	visitor.VisitSetFunctionNodeEnd(node)
}

func (node *SetFunctionNode) TypeTransform(s SymbolTypes) (Node, error) {
	var symbolNode Node = node.symbol
	err := transformTypes(s, &symbolNode)
	if err != nil {
		return node, err
	}
	newSymbolNode, ok := symbolNode.(SymbolNode)
	if !ok {
		return node, errors.Errorf("identifier symbol %v was transformed to non-identifier node: %v",
			node.symbol.Symbol(), reflect.TypeOf(symbolNode))
	}
	node.symbol = newSymbolNode

	if !node.IsCompare() {
		symbol := node.symbol
		var query Query
		subQuery, ok := symbol.(*subQueryNode)
		if ok {
			symbol = subQuery.symbol
			query = subQuery.query
		}
		if node.setFunction == SetFunctionCount {
			return &CountSetExprNode{
				symbol: symbol,
			}, nil
		}
		if node.setFunction == SetFunctionIsEmpty {
			return &IsEmptySetExprNode{
				symbol: symbol,
				query:  query,
			}, nil
		}
	}
	return node, nil
}

func (node *SetFunctionNode) String() string {
	return fmt.Sprintf("%v(%v)", node.setFunction, node.symbol)
}

func (node *SetFunctionNode) GetType() NodeType {
	switch node.setFunction {
	case SetFunctionCount:
		return NodeTypeInt64
	case SetFunctionIsEmpty:
		return NodeTypeBool
	default:
		return node.symbol.GetType()
	}
}

func (node *SetFunctionNode) IsCompare() bool {
	switch node.setFunction {
	case SetFunctionAllOf, SetFunctionAnyOf:
		return true
	default:
		return false
	}
}

func (node *SetFunctionNode) MoveUpTree(boolNode BoolNode) (BoolNode, error) {
	switch node.setFunction {
	case SetFunctionAllOf:
		return &AllOfSetExprNode{name: node.symbol.Symbol(), predicate: boolNode}, nil
	case SetFunctionAnyOf:
		return node.specializeSetAnyOf(boolNode)
	default:
		return nil, errors.Errorf("unhandled set function %v", node.setFunction)
	}
}

func (node *SetFunctionNode) specializeSetAnyOf(boolNode BoolNode) (BoolNode, error) {
	if seekOptimizable, ok := boolNode.(SeekOptimizableBoolNode); ok && seekOptimizable.IsSeekable() {
		return &AnyOfSetExprNode{
			name:              node.symbol.Symbol(),
			predicate:         boolNode,
			seekablePredicate: seekOptimizable,
		}, nil
	}
	return &AnyOfSetExprNode{name: node.symbol.Symbol(), predicate: boolNode}, nil
}

func (node *SetFunctionNode) IsConst() bool {
	return false
}

type UntypedNotExprNode struct {
	expr Node
}

func (node *UntypedNotExprNode) String() string {
	return fmt.Sprintf("not (%v)", node.expr.String())
}

func (node *UntypedNotExprNode) Accept(visitor Visitor) {
	visitor.VisitUntypedNotExprStart(node)
	node.expr.Accept(visitor)
	visitor.VisitUntypedNotExprEnd(node)
}

func (node *UntypedNotExprNode) GetType() NodeType {
	return NodeTypeBool
}

func (node *UntypedNotExprNode) EvalBool(_ Symbols) bool {
	pfxlog.Logger().Errorf("cannot evaluate transitory untyped not node %v", node)
	return false
}

func (node *UntypedNotExprNode) TypeTransformBool(s SymbolTypes) (BoolNode, error) {
	if err := transformTypes(s, &node.expr); err != nil {
		return node, err
	}

	boolNode, ok := node.expr.(BoolNode)
	if !ok {
		return node, errors.Errorf("not expr must wrap bool expr. contains %v", reflect.TypeOf(node.expr))
	}

	return &NotExprNode{expr: boolNode}, nil
}

func (node *UntypedNotExprNode) IsConst() bool {
	return node.expr.IsConst()
}
