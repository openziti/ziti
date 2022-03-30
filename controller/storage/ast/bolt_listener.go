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
	"reflect"
	"strconv"
	"strings"

	"github.com/antlr/antlr4/runtime/Go/antlr"
	zitiql "github.com/openziti/storage/zitiql"
	"github.com/pkg/errors"
)

type Stack struct {
	values []interface{}
}

func NewListener() *ToBoltListener {
	return &ToBoltListener{
		PrintRuleLocation: false,
		PrintChildren:     false,
		PrintStackOps:     false,
		stacks:            &Stack{},
		currentStack:      &Stack{},
		err:               nil,
	}
}

var _ zitiql.ZitiQlListener = (*ToBoltListener)(nil)

type ToBoltListener struct {
	LoggingListener
	PrintRuleLocation bool
	PrintChildren     bool
	PrintStackOps     bool
	stacks            *Stack
	currentStack      *Stack
	err               error
}

func (stack *Stack) push(val interface{}) {
	stack.values = append(stack.values, val)
}

func (stack *Stack) pop() (interface{}, error) {
	size := len(stack.values)
	if size == 0 {
		return nil, errors.New("stack is empty, cannot pop")
	}
	result := stack.values[size-1]
	stack.values = stack.values[:size-1]

	return result, nil
}

func (stack *Stack) peek() interface{} {
	size := len(stack.values)
	if size == 0 {
		return nil
	}
	return stack.values[size-1]
}

func (bl *ToBoltListener) HasError() bool {
	return bl.err != nil
}

func (bl *ToBoltListener) SetError(err error) {
	if bl.err == nil {
		bl.err = err
	}
}

func (bl *ToBoltListener) GetError() error {
	return bl.err
}

func (bl *ToBoltListener) enterGroup() {
	bl.stacks.push(bl.currentStack)
	bl.currentStack = &Stack{}

	if bl.PrintStackOps {
		fmt.Printf("entered group. new stack depth: %v\n", len(bl.stacks.values))
	}
}

func (bl *ToBoltListener) exitGroup() {
	lastStack, err := bl.stacks.pop()
	if err != nil {
		bl.SetError(err)
	} else {
		bl.currentStack = lastStack.(*Stack)
	}
	if bl.PrintStackOps {
		fmt.Printf("exited group. new stack depth: %v\n", len(bl.stacks.values))
	}
}

func (bl *ToBoltListener) pushStack(val interface{}) {
	if !bl.HasError() {
		bl.currentStack.push(val)

		if bl.PrintStackOps {
			fmt.Printf("pushed %v(%v) onto stack, new stack depth %v\n", reflect.TypeOf(val), val, len(bl.currentStack.values))
		}
	}
}

func (bl *ToBoltListener) popStack() interface{} {
	if bl.HasError() {
		return bl.err
	}
	result, err := bl.currentStack.pop()
	if err != nil {
		bl.SetError(err)
	}

	if bl.PrintStackOps {
		fmt.Printf("popped %v from stack. new stack depth: %v\n", result, len(bl.currentStack.values))
	}

	return result
}

func (bl *ToBoltListener) peekStack() interface{} {
	if bl.HasError() {
		return nil
	}
	return bl.currentStack.peek()
}

func (bl *ToBoltListener) popNode() Node {
	if bl.HasError() {
		return nil
	}
	val := bl.popStack()
	node, ok := val.(Node)
	if !ok {
		bl.SetError(errors.Errorf("expected Node on parse stack, but got %v", reflect.TypeOf(val)))
	}
	return node
}

func (bl *ToBoltListener) getQuery(symbols SymbolTypes) (Query, error) {
	if bl.HasError() {
		return nil, bl.err
	}

	node := bl.popNode()
	query, ok := node.(*untypedQueryNode)
	if !ok {
		return nil, errors.Errorf("unexpected result from query parsing. expected query, got %v", reflect.TypeOf(node))
	}

	if bl.HasError() {
		return nil, bl.err
	}

	var bQuery BoolNode = query
	err := PostProcess(symbols, &bQuery)
	if err != nil {
		return nil, err
	}

	if result, ok := bQuery.(Query); ok {
		return result, nil
	}
	return nil, errors.Errorf("unexpected query type %v", reflect.TypeOf(bQuery))
}

func (bl *ToBoltListener) popBinaryOperand() BinaryOp {
	if bl.HasError() {
		return 0
	}
	val := bl.popStack()
	op, ok := val.(BinaryOp)
	if !ok {
		bl.SetError(errors.Errorf("expected binary op on parse stack, but got %v", reflect.TypeOf(val)))
	}
	return op
}

func (bl *ToBoltListener) popSetFunction() SetFunction {
	if bl.HasError() {
		return 0
	}
	val := bl.popStack()
	op, ok := val.(SetFunction)
	if !ok {
		bl.SetError(errors.Errorf("expected set function (anyOf, allOf, noneOf) on parse stack, but got %v", reflect.TypeOf(val)))
	}
	return op
}

func (bl *ToBoltListener) popSymbolNode() SymbolNode {
	if bl.HasError() {
		return nil
	}
	val := bl.popStack()
	op, ok := val.(SymbolNode)
	if !ok {
		bl.SetError(errors.Errorf("expected identifier on parse stack, but got %v", reflect.TypeOf(val)))
	}
	return op
}

func (bl *ToBoltListener) VisitTerminal(node antlr.TerminalNode) {
	bl.printDebug(node)

	if bl.HasError() {
		return
	}

	if bl.PrintRuleLocation {
		fmt.Printf("Visiting terminal of type %v - %v\n", node.GetSymbol().GetText(), node.GetSymbol().GetTokenType())
	}

	switch node.GetSymbol().GetTokenType() {
	case zitiql.ZitiQlLexerBOOL:
		bl.appendBoolNode(node.GetText())
	case zitiql.ZitiQlLexerDATETIME:
		bl.appendDateTimeNode(node.GetText())
	case zitiql.ZitiQlLexerIDENTIFIER:
		bl.pushStack(&UntypedSymbolNode{symbol: node.GetText()})
	case zitiql.ZitiQlLexerNULL:
		bl.pushStack(NullConstNode{})
	case zitiql.ZitiQlLexerNUMBER:
		bl.appendNumberNode(node.GetText())
	case zitiql.ZitiQlLexerNONE:
		bl.pushStack(&Int64ConstNode{value: int64(-1)}) // LIMIT NONE gets turned into marker value -1
	case zitiql.ZitiQlLexerSTRING:
		result := zitiql.ParseZqlString(node.GetText())
		bl.pushStack(&StringConstNode{value: result})
	case zitiql.ZitiQlLexerEQ, zitiql.ZitiQlLexerGT, zitiql.ZitiQlLexerLT:
		bl.pushStack(binaryOpValues[node.GetText()])
	case zitiql.ZitiQlLexerIN:
		if strings.Contains(strings.ToLower(node.GetText()), "not") {
			bl.pushStack(BinaryOpNotIn)
		} else {
			bl.pushStack(BinaryOpIn)
		}
	case zitiql.ZitiQlLexerBETWEEN:
		if strings.Contains(strings.ToLower(node.GetText()), "not") {
			bl.pushStack(BinaryOpNotBetween)
		} else {
			bl.pushStack(BinaryOpBetween)
		}
	case zitiql.ZitiQlLexerCONTAINS:
		if strings.Contains(strings.ToLower(node.GetText()), "not") {
			bl.pushStack(BinaryOpNotContains)
		} else {
			bl.pushStack(BinaryOpContains)
		}
	case zitiql.ZitiQlLexerALL_OF:
		bl.pushStack(SetFunctionAllOf)
	case zitiql.ZitiQlLexerANY_OF:
		bl.pushStack(SetFunctionAnyOf)
	case zitiql.ZitiQlLexerCOUNT:
		bl.pushStack(SetFunctionCount)
	case zitiql.ZitiQlLexerISEMPTY:
		bl.pushStack(SetFunctionIsEmpty)
	case zitiql.ZitiQlLexerASC:
		bl.pushStack(SortAscending)
	case zitiql.ZitiQlLexerDESC:
		bl.pushStack(SortDescending)
	}
}

func (bl *ToBoltListener) appendBoolNode(text string) {
	val, err := strconv.ParseBool(strings.ToLower(text))
	if err != nil {
		bl.SetError(err)
		return
	}
	bl.pushStack(&BoolConstNode{value: val})
}

func (bl *ToBoltListener) appendNumberNode(text string) {
	intVal, intErr := strconv.ParseInt(text, 10, 64)
	if intErr == nil {
		node := &Int64ConstNode{value: intVal}
		bl.pushStack(node)
		return
	}

	floatVal, floatErr := strconv.ParseFloat(text, 64)

	if floatErr != nil {
		bl.SetError(errors.Errorf("could not parse number as float or int: int(%s) and float(%s)", intErr, floatErr))
		return
	}

	node := &Float64ConstNode{value: floatVal}
	bl.pushStack(node)
}

func (bl *ToBoltListener) appendDateTimeNode(text string) {
	t, err := zitiql.ParseZqlDatetime(text)
	if err != nil {
		bl.SetError(err)
		return
	}
	bl.pushStack(&DatetimeConstNode{t})
}

func (bl *ToBoltListener) EnterStringArray(c *zitiql.StringArrayContext) {
	bl.printDebug(c)
	bl.enterGroup()
}

func (bl *ToBoltListener) EnterNumberArray(c *zitiql.NumberArrayContext) {
	bl.printDebug(c)
	bl.enterGroup()
}

func (bl *ToBoltListener) EnterDatetimeArray(c *zitiql.DatetimeArrayContext) {
	bl.printDebug(c)
	bl.enterGroup()
}

func (bl *ToBoltListener) ExitStringArray(c *zitiql.StringArrayContext) {
	bl.printDebug(c)
	if bl.HasError() {
		return
	}
	arrayNode := &StringArrayNode{}
	for len(bl.currentStack.values) > 0 {
		node := bl.popNode()
		typedNode, ok := node.(StringNode)
		if !ok {
			bl.SetError(errors.Errorf("unexpected value of type %v in string array", nodeTypeNames[node.GetType()]))
			return
		}
		arrayNode.values = append(arrayNode.values, typedNode)
	}
	bl.exitGroup()
	bl.pushStack(arrayNode)
}

func (bl *ToBoltListener) ExitNumberArray(c *zitiql.NumberArrayContext) {
	bl.printDebug(c)
	if bl.HasError() {
		return
	}
	var nodes []Node
	allInt := true
	for len(bl.currentStack.values) > 0 {
		node := bl.popNode()
		intNode, ok := node.(Int64Node)
		if !ok {
			floatNode, ok := node.(Float64Node)
			if !ok {
				bl.SetError(errors.Errorf("unexpected value of type %v in number array", nodeTypeNames[node.GetType()]))
				return
			}
			allInt = false
			nodes = append(nodes, floatNode)
		} else {
			nodes = append(nodes, intNode)
		}
	}

	bl.exitGroup()

	if allInt {
		arrayNode := &Int64ArrayNode{}
		for _, node := range nodes {
			arrayNode.values = append(arrayNode.values, node.(Int64Node))
		}
		bl.pushStack(arrayNode)
	} else {
		arrayNode := &Float64ArrayNode{}
		for _, node := range nodes {
			if intNode, ok := node.(Int64Node); ok {
				arrayNode.values = append(arrayNode.values, intNode.ToFloat64())
			} else {
				arrayNode.values = append(arrayNode.values, node.(Float64Node))
			}
		}
		bl.pushStack(arrayNode)
	}
}

func (bl *ToBoltListener) ExitDatetimeArray(c *zitiql.DatetimeArrayContext) {
	bl.printDebug(c)
	if bl.HasError() {
		return
	}
	arrayNode := &DatetimeArrayNode{}
	for len(bl.currentStack.values) > 0 {
		node := bl.popNode()
		typedNode, ok := node.(DatetimeNode)
		if !ok {
			bl.SetError(errors.Errorf("unexpected value of type %v in date array", nodeTypeNames[node.GetType()]))
			return
		}
		arrayNode.values = append(arrayNode.values, typedNode)
	}
	bl.exitGroup()
	bl.pushStack(arrayNode)
}

func (bl *ToBoltListener) ExitOrExpr(c *zitiql.OrExprContext) {
	bl.printDebug(c)
	right := bl.popNode()
	left := bl.popNode()

	if !bl.HasError() {
		bl.pushStack(&BooleanLogicExprNode{left: left, right: right, op: OrOp})
	}
}

func (bl *ToBoltListener) ExitAndExpr(c *zitiql.AndExprContext) {
	bl.printDebug(c)

	right := bl.popNode()
	left := bl.popNode()

	if !bl.HasError() {
		bl.pushStack(&BooleanLogicExprNode{left: left, right: right, op: AndOp})
	}
}

func (bl *ToBoltListener) ExitInStringArrayOp(c *zitiql.InStringArrayOpContext) {
	bl.printDebug(c)
	bl.exitInArrayOp()
}

func (bl *ToBoltListener) ExitInNumberArrayOp(c *zitiql.InNumberArrayOpContext) {
	bl.printDebug(c)
	bl.exitInArrayOp()
}

func (bl *ToBoltListener) ExitInDatetimeArrayOp(c *zitiql.InDatetimeArrayOpContext) {
	bl.printDebug(c)
	bl.exitInArrayOp()
}

func (bl *ToBoltListener) exitInArrayOp() {
	right := bl.popNode()
	op := bl.popBinaryOperand()
	left := bl.popNode()

	if !bl.HasError() {
		node := NewInArrayExprNode(left, right)

		if op == BinaryOpIn {
			bl.pushStack(node)
		} else if op == BinaryOpNotIn {
			bl.pushStack(&NotExprNode{expr: node})
		} else {
			bl.SetError(errors.Errorf("Unexpected operation: %v", op))
		}
	}
}

func (bl *ToBoltListener) ExitBetweenNumberOp(c *zitiql.BetweenNumberOpContext) {
	bl.printDebug(c)
	bl.exitBetweenOp()
}

func (bl *ToBoltListener) ExitBetweenDateOp(c *zitiql.BetweenDateOpContext) {
	bl.printDebug(c)
	bl.exitBetweenOp()
}

func (bl *ToBoltListener) exitBetweenOp() {
	upper := bl.popNode()
	lower := bl.popNode()
	op := bl.popBinaryOperand()
	left := bl.popNode()

	if !bl.HasError() {
		node := &BetweenExprNode{left: left, lower: lower, upper: upper}

		if op == BinaryOpBetween {
			bl.pushStack(node)
		} else if op == BinaryOpNotBetween {
			bl.pushStack(&NotExprNode{expr: node})
		} else {
			bl.SetError(errors.Errorf("Unexpected operation: %v", op))
		}
	}
}

func (bl *ToBoltListener) ExitBinaryLessThanStringOp(c *zitiql.BinaryLessThanStringOpContext) {
	bl.printDebug(c)
	bl.ExitBinaryOp()
}

func (bl *ToBoltListener) ExitBinaryGreaterThanStringOp(c *zitiql.BinaryGreaterThanStringOpContext) {
	bl.printDebug(c)
	bl.ExitBinaryOp()
}

func (bl *ToBoltListener) ExitBinaryLessThanNumberOp(c *zitiql.BinaryLessThanNumberOpContext) {
	bl.printDebug(c)
	bl.ExitBinaryOp()
}

func (bl *ToBoltListener) ExitBinaryLessThanDatetimeOp(c *zitiql.BinaryLessThanDatetimeOpContext) {
	bl.printDebug(c)
	bl.ExitBinaryOp()
}

func (bl *ToBoltListener) ExitBinaryGreaterThanNumberOp(c *zitiql.BinaryGreaterThanNumberOpContext) {
	bl.printDebug(c)
	bl.ExitBinaryOp()
}

func (bl *ToBoltListener) ExitBinaryGreaterThanDatetimeOp(c *zitiql.BinaryGreaterThanDatetimeOpContext) {
	bl.printDebug(c)
	bl.ExitBinaryOp()
}

func (bl *ToBoltListener) ExitBinaryEqualToStringOp(c *zitiql.BinaryEqualToStringOpContext) {
	bl.printDebug(c)
	bl.ExitBinaryOp()
}

func (bl *ToBoltListener) ExitBinaryEqualToNumberOp(c *zitiql.BinaryEqualToNumberOpContext) {
	bl.printDebug(c)
	bl.ExitBinaryOp()
}

func (bl *ToBoltListener) ExitBinaryEqualToDatetimeOp(c *zitiql.BinaryEqualToDatetimeOpContext) {
	bl.printDebug(c)
	bl.ExitBinaryOp()
}

func (bl *ToBoltListener) ExitBinaryEqualToBoolOp(c *zitiql.BinaryEqualToBoolOpContext) {
	bl.printDebug(c)
	bl.ExitBinaryOp()
}

func (bl *ToBoltListener) ExitBinaryEqualToNullOp(c *zitiql.BinaryEqualToNullOpContext) {
	bl.printDebug(c)
	bl.ExitBinaryOp()
}

func (bl *ToBoltListener) ExitBinaryContainsOp(c *zitiql.BinaryContainsOpContext) {
	bl.printDebug(c)
	bl.ExitBinaryOp()
}

func (bl *ToBoltListener) ExitBinaryOp() {
	right := bl.popNode()
	op := bl.popBinaryOperand()
	left := bl.popNode()

	if !bl.HasError() {
		bl.pushStack(&BinaryExprNode{
			left:  left,
			right: right,
			op:    op,
		})
	}
}

func (bl *ToBoltListener) ExitSetFunctionExpr(c *zitiql.SetFunctionExprContext) {
	bl.printDebug(c)
	bl.pushSetFunction()
}

func (bl *ToBoltListener) ExitIsEmptyFunction(c *zitiql.IsEmptyFunctionContext) {
	bl.printDebug(c)
	bl.pushSetFunction()
}

func (bl *ToBoltListener) pushSetFunction() {
	symbol := bl.popSymbolNode()
	op := bl.popSetFunction()

	if !bl.HasError() {
		bl.pushStack(&SetFunctionNode{
			setFunction: op,
			symbol:      symbol,
		})
	}
}

func (bl *ToBoltListener) EnterSortByExpr(c *zitiql.SortByExprContext) {
	bl.printDebug(c)
	bl.enterGroup()
}

func (bl *ToBoltListener) ExitSortByExpr(c *zitiql.SortByExprContext) {
	bl.printDebug(c)
	result := &SortByNode{
		SortFields: make([]*SortFieldNode, len(bl.currentStack.values)),
	}

	for idx, node := range bl.currentStack.values {
		sortField, ok := node.(*SortFieldNode)
		if !ok {
			bl.SetError(errors.Errorf("unexpected value of type %v in sort by", reflect.TypeOf(node)))
			return
		}
		result.SortFields[idx] = sortField
	}
	bl.exitGroup()
	bl.pushStack(result)
}

func (bl *ToBoltListener) ExitSortFieldExpr(c *zitiql.SortFieldExprContext) {
	bl.printDebug(c)
	direction, ok := bl.peekStack().(SortDirection)
	if !ok {
		direction = SortAscending
	} else {
		bl.popStack()
	}
	idNode := bl.popSymbolNode()
	if !bl.HasError() {
		sortField := &SortFieldNode{
			symbol:      idNode,
			isAscending: bool(direction),
		}
		bl.pushStack(sortField)
	}
}

func (bl *ToBoltListener) ExitSkipExpr(c *zitiql.SkipExprContext) {
	bl.printDebug(c)
	val := bl.popNode()
	if bl.HasError() {
		return
	}
	skip, ok := val.(*Int64ConstNode)
	if !ok {
		bl.SetError(errors.Errorf("expected integer value for skip, but got %v", reflect.TypeOf(val)))
		return
	}
	bl.pushStack(&SkipExprNode{Int64ConstNode: *skip})
}

func (bl *ToBoltListener) ExitLimitExpr(c *zitiql.LimitExprContext) {
	bl.printDebug(c)
	val := bl.popNode()
	if bl.HasError() {
		return
	}
	limit, ok := val.(*Int64ConstNode)
	if !ok {
		bl.SetError(errors.Errorf("expected integer value for limit, but got %v", reflect.TypeOf(val)))
		return
	}
	bl.pushStack(&LimitExprNode{Int64ConstNode: *limit})
}

func (bl *ToBoltListener) ExitQueryStmt(c *zitiql.QueryStmtContext) {
	bl.printDebug(c)

	result := &untypedQueryNode{}

	if limit, ok := bl.peekStack().(*LimitExprNode); ok {
		result.limit = limit
		bl.popStack()
	}

	if skip, ok := bl.peekStack().(*SkipExprNode); ok {
		result.skip = skip
		bl.popStack()
	}

	if sortBy, ok := bl.peekStack().(*SortByNode); ok {
		result.sortBy = sortBy
		bl.popStack()
	}

	if _, ok := bl.peekStack().(Node); ok {
		result.predicate = bl.popNode()
	} else {
		result.predicate = BoolNodeTrue
	}

	if !bl.HasError() {
		bl.pushStack(result)
	}
}

func (bl *ToBoltListener) ExitSubQuery(c *zitiql.SubQueryContext) {
	bl.printDebug(c)
	queryNode := bl.popNode()
	node := bl.popNode()
	if bl.HasError() {
		return
	}
	symbolNode, ok := node.(SymbolNode)
	if !ok {
		bl.SetError(errors.Errorf("subquery node must have identifier symbol. had %v instead", reflect.TypeOf(node)))
		return
	}

	if !bl.HasError() {
		subQueryNode := &UntypedSubQueryNode{
			symbol: symbolNode,
			query:  queryNode,
		}
		bl.pushStack(subQueryNode)
	}
}

func (bl *ToBoltListener) ExitNotExpr(c *zitiql.NotExprContext) {
	bl.printDebug(c)
	expr := bl.popNode()
	if !bl.HasError() {
		bl.pushStack(&UntypedNotExprNode{expr: expr})
	}
}
