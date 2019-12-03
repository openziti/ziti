/*
	Copyright 2019 Netfoundry, Inc.

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

// Code generated from ZitiQl.g4 by ANTLR 4.7.1. DO NOT EDIT.

package zitiql // ZitiQl
import "github.com/antlr/antlr4/runtime/Go/antlr"

// BaseZitiQlListener is a complete listener for a parse tree produced by ZitiQlParser.
type BaseZitiQlListener struct{}

var _ ZitiQlListener = &BaseZitiQlListener{}

// VisitTerminal is called when a terminal node is visited.
func (s *BaseZitiQlListener) VisitTerminal(node antlr.TerminalNode) {}

// VisitErrorNode is called when an error node is visited.
func (s *BaseZitiQlListener) VisitErrorNode(node antlr.ErrorNode) {}

// EnterEveryRule is called when any rule is entered.
func (s *BaseZitiQlListener) EnterEveryRule(ctx antlr.ParserRuleContext) {}

// ExitEveryRule is called when any rule is exited.
func (s *BaseZitiQlListener) ExitEveryRule(ctx antlr.ParserRuleContext) {}

// EnterString_array is called when production string_array is entered.
func (s *BaseZitiQlListener) EnterString_array(ctx *String_arrayContext) {}

// ExitString_array is called when production string_array is exited.
func (s *BaseZitiQlListener) ExitString_array(ctx *String_arrayContext) {}

// EnterNumber_array is called when production number_array is entered.
func (s *BaseZitiQlListener) EnterNumber_array(ctx *Number_arrayContext) {}

// ExitNumber_array is called when production number_array is exited.
func (s *BaseZitiQlListener) ExitNumber_array(ctx *Number_arrayContext) {}

// EnterDatetime_array is called when production datetime_array is entered.
func (s *BaseZitiQlListener) EnterDatetime_array(ctx *Datetime_arrayContext) {}

// ExitDatetime_array is called when production datetime_array is exited.
func (s *BaseZitiQlListener) ExitDatetime_array(ctx *Datetime_arrayContext) {}

// EnterEnd is called when production End is entered.
func (s *BaseZitiQlListener) EnterEnd(ctx *EndContext) {}

// ExitEnd is called when production End is exited.
func (s *BaseZitiQlListener) ExitEnd(ctx *EndContext) {}

// EnterGroup is called when production Group is entered.
func (s *BaseZitiQlListener) EnterGroup(ctx *GroupContext) {}

// ExitGroup is called when production Group is exited.
func (s *BaseZitiQlListener) ExitGroup(ctx *GroupContext) {}

// EnterOrConjunction is called when production OrConjunction is entered.
func (s *BaseZitiQlListener) EnterOrConjunction(ctx *OrConjunctionContext) {}

// ExitOrConjunction is called when production OrConjunction is exited.
func (s *BaseZitiQlListener) ExitOrConjunction(ctx *OrConjunctionContext) {}

// EnterOperationOp is called when production OperationOp is entered.
func (s *BaseZitiQlListener) EnterOperationOp(ctx *OperationOpContext) {}

// ExitOperationOp is called when production OperationOp is exited.
func (s *BaseZitiQlListener) ExitOperationOp(ctx *OperationOpContext) {}

// EnterAndConjunction is called when production AndConjunction is entered.
func (s *BaseZitiQlListener) EnterAndConjunction(ctx *AndConjunctionContext) {}

// ExitAndConjunction is called when production AndConjunction is exited.
func (s *BaseZitiQlListener) ExitAndConjunction(ctx *AndConjunctionContext) {}

// EnterInStringArrayOp is called when production InStringArrayOp is entered.
func (s *BaseZitiQlListener) EnterInStringArrayOp(ctx *InStringArrayOpContext) {}

// ExitInStringArrayOp is called when production InStringArrayOp is exited.
func (s *BaseZitiQlListener) ExitInStringArrayOp(ctx *InStringArrayOpContext) {}

// EnterInNumberArrayOp is called when production InNumberArrayOp is entered.
func (s *BaseZitiQlListener) EnterInNumberArrayOp(ctx *InNumberArrayOpContext) {}

// ExitInNumberArrayOp is called when production InNumberArrayOp is exited.
func (s *BaseZitiQlListener) ExitInNumberArrayOp(ctx *InNumberArrayOpContext) {}

// EnterInDatetimeArrayOp is called when production InDatetimeArrayOp is entered.
func (s *BaseZitiQlListener) EnterInDatetimeArrayOp(ctx *InDatetimeArrayOpContext) {}

// ExitInDatetimeArrayOp is called when production InDatetimeArrayOp is exited.
func (s *BaseZitiQlListener) ExitInDatetimeArrayOp(ctx *InDatetimeArrayOpContext) {}

// EnterBetweenNumberOp is called when production BetweenNumberOp is entered.
func (s *BaseZitiQlListener) EnterBetweenNumberOp(ctx *BetweenNumberOpContext) {}

// ExitBetweenNumberOp is called when production BetweenNumberOp is exited.
func (s *BaseZitiQlListener) ExitBetweenNumberOp(ctx *BetweenNumberOpContext) {}

// EnterBetweenDateOp is called when production BetweenDateOp is entered.
func (s *BaseZitiQlListener) EnterBetweenDateOp(ctx *BetweenDateOpContext) {}

// ExitBetweenDateOp is called when production BetweenDateOp is exited.
func (s *BaseZitiQlListener) ExitBetweenDateOp(ctx *BetweenDateOpContext) {}

// EnterBinaryLessThanNumberOp is called when production BinaryLessThanNumberOp is entered.
func (s *BaseZitiQlListener) EnterBinaryLessThanNumberOp(ctx *BinaryLessThanNumberOpContext) {}

// ExitBinaryLessThanNumberOp is called when production BinaryLessThanNumberOp is exited.
func (s *BaseZitiQlListener) ExitBinaryLessThanNumberOp(ctx *BinaryLessThanNumberOpContext) {}

// EnterBinaryLessThanDatetimeOp is called when production BinaryLessThanDatetimeOp is entered.
func (s *BaseZitiQlListener) EnterBinaryLessThanDatetimeOp(ctx *BinaryLessThanDatetimeOpContext) {}

// ExitBinaryLessThanDatetimeOp is called when production BinaryLessThanDatetimeOp is exited.
func (s *BaseZitiQlListener) ExitBinaryLessThanDatetimeOp(ctx *BinaryLessThanDatetimeOpContext) {}

// EnterBinaryGreaterThanNumberOp is called when production BinaryGreaterThanNumberOp is entered.
func (s *BaseZitiQlListener) EnterBinaryGreaterThanNumberOp(ctx *BinaryGreaterThanNumberOpContext) {}

// ExitBinaryGreaterThanNumberOp is called when production BinaryGreaterThanNumberOp is exited.
func (s *BaseZitiQlListener) ExitBinaryGreaterThanNumberOp(ctx *BinaryGreaterThanNumberOpContext) {}

// EnterBinaryGreaterThanDatetimeOp is called when production BinaryGreaterThanDatetimeOp is entered.
func (s *BaseZitiQlListener) EnterBinaryGreaterThanDatetimeOp(ctx *BinaryGreaterThanDatetimeOpContext) {
}

// ExitBinaryGreaterThanDatetimeOp is called when production BinaryGreaterThanDatetimeOp is exited.
func (s *BaseZitiQlListener) ExitBinaryGreaterThanDatetimeOp(ctx *BinaryGreaterThanDatetimeOpContext) {
}

// EnterBinaryEqualToStringOp is called when production BinaryEqualToStringOp is entered.
func (s *BaseZitiQlListener) EnterBinaryEqualToStringOp(ctx *BinaryEqualToStringOpContext) {}

// ExitBinaryEqualToStringOp is called when production BinaryEqualToStringOp is exited.
func (s *BaseZitiQlListener) ExitBinaryEqualToStringOp(ctx *BinaryEqualToStringOpContext) {}

// EnterBinaryEqualToNumberOp is called when production BinaryEqualToNumberOp is entered.
func (s *BaseZitiQlListener) EnterBinaryEqualToNumberOp(ctx *BinaryEqualToNumberOpContext) {}

// ExitBinaryEqualToNumberOp is called when production BinaryEqualToNumberOp is exited.
func (s *BaseZitiQlListener) ExitBinaryEqualToNumberOp(ctx *BinaryEqualToNumberOpContext) {}

// EnterBinaryEqualToDatetimeOp is called when production BinaryEqualToDatetimeOp is entered.
func (s *BaseZitiQlListener) EnterBinaryEqualToDatetimeOp(ctx *BinaryEqualToDatetimeOpContext) {}

// ExitBinaryEqualToDatetimeOp is called when production BinaryEqualToDatetimeOp is exited.
func (s *BaseZitiQlListener) ExitBinaryEqualToDatetimeOp(ctx *BinaryEqualToDatetimeOpContext) {}

// EnterBinaryEqualToBoolOp is called when production BinaryEqualToBoolOp is entered.
func (s *BaseZitiQlListener) EnterBinaryEqualToBoolOp(ctx *BinaryEqualToBoolOpContext) {}

// ExitBinaryEqualToBoolOp is called when production BinaryEqualToBoolOp is exited.
func (s *BaseZitiQlListener) ExitBinaryEqualToBoolOp(ctx *BinaryEqualToBoolOpContext) {}

// EnterBinaryEqualToNullOp is called when production BinaryEqualToNullOp is entered.
func (s *BaseZitiQlListener) EnterBinaryEqualToNullOp(ctx *BinaryEqualToNullOpContext) {}

// ExitBinaryEqualToNullOp is called when production BinaryEqualToNullOp is exited.
func (s *BaseZitiQlListener) ExitBinaryEqualToNullOp(ctx *BinaryEqualToNullOpContext) {}

// EnterBinaryContainsOp is called when production BinaryContainsOp is entered.
func (s *BaseZitiQlListener) EnterBinaryContainsOp(ctx *BinaryContainsOpContext) {}

// ExitBinaryContainsOp is called when production BinaryContainsOp is exited.
func (s *BaseZitiQlListener) ExitBinaryContainsOp(ctx *BinaryContainsOpContext) {}
