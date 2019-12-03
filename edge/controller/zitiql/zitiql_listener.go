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

// ZitiQlListener is a complete listener for a parse tree produced by ZitiQlParser.
type ZitiQlListener interface {
	antlr.ParseTreeListener

	// EnterString_array is called when entering the string_array production.
	EnterString_array(c *String_arrayContext)

	// EnterNumber_array is called when entering the number_array production.
	EnterNumber_array(c *Number_arrayContext)

	// EnterDatetime_array is called when entering the datetime_array production.
	EnterDatetime_array(c *Datetime_arrayContext)

	// EnterEnd is called when entering the End production.
	EnterEnd(c *EndContext)

	// EnterGroup is called when entering the Group production.
	EnterGroup(c *GroupContext)

	// EnterOrConjunction is called when entering the OrConjunction production.
	EnterOrConjunction(c *OrConjunctionContext)

	// EnterOperationOp is called when entering the OperationOp production.
	EnterOperationOp(c *OperationOpContext)

	// EnterAndConjunction is called when entering the AndConjunction production.
	EnterAndConjunction(c *AndConjunctionContext)

	// EnterInStringArrayOp is called when entering the InStringArrayOp production.
	EnterInStringArrayOp(c *InStringArrayOpContext)

	// EnterInNumberArrayOp is called when entering the InNumberArrayOp production.
	EnterInNumberArrayOp(c *InNumberArrayOpContext)

	// EnterInDatetimeArrayOp is called when entering the InDatetimeArrayOp production.
	EnterInDatetimeArrayOp(c *InDatetimeArrayOpContext)

	// EnterBetweenNumberOp is called when entering the BetweenNumberOp production.
	EnterBetweenNumberOp(c *BetweenNumberOpContext)

	// EnterBetweenDateOp is called when entering the BetweenDateOp production.
	EnterBetweenDateOp(c *BetweenDateOpContext)

	// EnterBinaryLessThanNumberOp is called when entering the BinaryLessThanNumberOp production.
	EnterBinaryLessThanNumberOp(c *BinaryLessThanNumberOpContext)

	// EnterBinaryLessThanDatetimeOp is called when entering the BinaryLessThanDatetimeOp production.
	EnterBinaryLessThanDatetimeOp(c *BinaryLessThanDatetimeOpContext)

	// EnterBinaryGreaterThanNumberOp is called when entering the BinaryGreaterThanNumberOp production.
	EnterBinaryGreaterThanNumberOp(c *BinaryGreaterThanNumberOpContext)

	// EnterBinaryGreaterThanDatetimeOp is called when entering the BinaryGreaterThanDatetimeOp production.
	EnterBinaryGreaterThanDatetimeOp(c *BinaryGreaterThanDatetimeOpContext)

	// EnterBinaryEqualToStringOp is called when entering the BinaryEqualToStringOp production.
	EnterBinaryEqualToStringOp(c *BinaryEqualToStringOpContext)

	// EnterBinaryEqualToNumberOp is called when entering the BinaryEqualToNumberOp production.
	EnterBinaryEqualToNumberOp(c *BinaryEqualToNumberOpContext)

	// EnterBinaryEqualToDatetimeOp is called when entering the BinaryEqualToDatetimeOp production.
	EnterBinaryEqualToDatetimeOp(c *BinaryEqualToDatetimeOpContext)

	// EnterBinaryEqualToBoolOp is called when entering the BinaryEqualToBoolOp production.
	EnterBinaryEqualToBoolOp(c *BinaryEqualToBoolOpContext)

	// EnterBinaryEqualToNullOp is called when entering the BinaryEqualToNullOp production.
	EnterBinaryEqualToNullOp(c *BinaryEqualToNullOpContext)

	// EnterBinaryContainsOp is called when entering the BinaryContainsOp production.
	EnterBinaryContainsOp(c *BinaryContainsOpContext)

	// ExitString_array is called when exiting the string_array production.
	ExitString_array(c *String_arrayContext)

	// ExitNumber_array is called when exiting the number_array production.
	ExitNumber_array(c *Number_arrayContext)

	// ExitDatetime_array is called when exiting the datetime_array production.
	ExitDatetime_array(c *Datetime_arrayContext)

	// ExitEnd is called when exiting the End production.
	ExitEnd(c *EndContext)

	// ExitGroup is called when exiting the Group production.
	ExitGroup(c *GroupContext)

	// ExitOrConjunction is called when exiting the OrConjunction production.
	ExitOrConjunction(c *OrConjunctionContext)

	// ExitOperationOp is called when exiting the OperationOp production.
	ExitOperationOp(c *OperationOpContext)

	// ExitAndConjunction is called when exiting the AndConjunction production.
	ExitAndConjunction(c *AndConjunctionContext)

	// ExitInStringArrayOp is called when exiting the InStringArrayOp production.
	ExitInStringArrayOp(c *InStringArrayOpContext)

	// ExitInNumberArrayOp is called when exiting the InNumberArrayOp production.
	ExitInNumberArrayOp(c *InNumberArrayOpContext)

	// ExitInDatetimeArrayOp is called when exiting the InDatetimeArrayOp production.
	ExitInDatetimeArrayOp(c *InDatetimeArrayOpContext)

	// ExitBetweenNumberOp is called when exiting the BetweenNumberOp production.
	ExitBetweenNumberOp(c *BetweenNumberOpContext)

	// ExitBetweenDateOp is called when exiting the BetweenDateOp production.
	ExitBetweenDateOp(c *BetweenDateOpContext)

	// ExitBinaryLessThanNumberOp is called when exiting the BinaryLessThanNumberOp production.
	ExitBinaryLessThanNumberOp(c *BinaryLessThanNumberOpContext)

	// ExitBinaryLessThanDatetimeOp is called when exiting the BinaryLessThanDatetimeOp production.
	ExitBinaryLessThanDatetimeOp(c *BinaryLessThanDatetimeOpContext)

	// ExitBinaryGreaterThanNumberOp is called when exiting the BinaryGreaterThanNumberOp production.
	ExitBinaryGreaterThanNumberOp(c *BinaryGreaterThanNumberOpContext)

	// ExitBinaryGreaterThanDatetimeOp is called when exiting the BinaryGreaterThanDatetimeOp production.
	ExitBinaryGreaterThanDatetimeOp(c *BinaryGreaterThanDatetimeOpContext)

	// ExitBinaryEqualToStringOp is called when exiting the BinaryEqualToStringOp production.
	ExitBinaryEqualToStringOp(c *BinaryEqualToStringOpContext)

	// ExitBinaryEqualToNumberOp is called when exiting the BinaryEqualToNumberOp production.
	ExitBinaryEqualToNumberOp(c *BinaryEqualToNumberOpContext)

	// ExitBinaryEqualToDatetimeOp is called when exiting the BinaryEqualToDatetimeOp production.
	ExitBinaryEqualToDatetimeOp(c *BinaryEqualToDatetimeOpContext)

	// ExitBinaryEqualToBoolOp is called when exiting the BinaryEqualToBoolOp production.
	ExitBinaryEqualToBoolOp(c *BinaryEqualToBoolOpContext)

	// ExitBinaryEqualToNullOp is called when exiting the BinaryEqualToNullOp production.
	ExitBinaryEqualToNullOp(c *BinaryEqualToNullOpContext)

	// ExitBinaryContainsOp is called when exiting the BinaryContainsOp production.
	ExitBinaryContainsOp(c *BinaryContainsOpContext)
}
