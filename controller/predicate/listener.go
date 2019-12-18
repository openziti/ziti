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

package predicate

import (
	"fmt"
	"github.com/antlr/antlr4/runtime/Go/antlr"
	"github.com/netfoundry/ziti-edge/controller/zitiql"
	"gopkg.in/Masterminds/squirrel.v1"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type ToSquirrelListener struct {
	*zitiql.BaseZitiQlListener
	parseState        *parseState
	Predicate         squirrel.Sqlizer
	PrintRuleLocation bool
	PrintChildren     bool
	IdentifierOps     IdentifierOps
	identifierMap     *IdentifierMap
	IdentifierErrors  []zitiql.ParseError
}

func caseInsensitiveContains(s, substr string) bool {
	s, substr = strings.ToUpper(s), strings.ToUpper(substr)
	return strings.Contains(s, substr)
}

func NewSquirrelListener() *ToSquirrelListener {
	return &ToSquirrelListener{
		identifierMap:     &IdentifierMap{},
		parseState:        newParseState(),
		PrintRuleLocation: false,
		PrintChildren:     false,
	}
}

func NewSquirrelListenerWithMap(identifierMap *IdentifierMap) *ToSquirrelListener {
	return &ToSquirrelListener{
		identifierMap:     identifierMap,
		parseState:        newParseState(),
		PrintRuleLocation: false,
		PrintChildren:     false,
	}
}

func NewZitiqlParseError(tn *antlr.TerminalNode, msg string) zitiql.ParseError {
	n := *tn

	return zitiql.ParseError{
		Symbol:  n.GetText(),
		Line:    n.GetSymbol().GetLine(),
		Column:  n.GetSymbol().GetColumn(),
		Message: msg,
	}
}

func (z *ToSquirrelListener) translateIdentifier(tn *antlr.TerminalNode) string {
	identifier := (*tn).GetText()
	if trans, ok := (*z.identifierMap)[identifier]; ok {
		return trans
	}

	z.IdentifierErrors = append(z.IdentifierErrors, NewZitiqlParseError(tn, "Invalid identifier"))

	return identifier
}

func (z *ToSquirrelListener) printRuleLocationWithSkip(s int) {
	if z.PrintRuleLocation {
		pc, _, _, _ := runtime.Caller(s)
		f := runtime.FuncForPC(pc)
		s := strings.Split(f.Name(), ".")
		println(s[len(s)-1])
	}
}

func (z *ToSquirrelListener) printChildren(tree antlr.ParseTree) {
	if z.PrintChildren {
		fmt.Printf("children for: %s\n", tree.GetText())

		for i, c := range tree.GetChildren() {
			fmt.Printf("-- %d: %s\n", i, c.(antlr.ParseTree).GetText())
		}
	}
}

func (z *ToSquirrelListener) printDebug(tree antlr.ParseTree) {
	z.printRuleLocationWithSkip(2)
	z.printChildren(tree)
}

// EnterEnd is called when entering the End production.
func (z *ToSquirrelListener) EnterEnd(c *zitiql.EndContext) { z.printDebug(c) }

// EnterGroup is called when entering the Group production.
func (z *ToSquirrelListener) EnterGroup(c *zitiql.GroupContext) {
	z.printDebug(c)
	z.parseState.EnterGroup()
}

func (z *ToSquirrelListener) HandleError(err error) {
	panic(err)
}

// EnterOrConjunction is called when entering the OrConjunction production.
func (z *ToSquirrelListener) EnterOrConjunction(c *zitiql.OrConjunctionContext) {
	z.printDebug(c)
	z.parseState.AddOr()
}

// EnterOperationOp is called when entering the OperationOp production.
func (z *ToSquirrelListener) EnterOperationOp(c *zitiql.OperationOpContext) { z.printDebug(c) }

// EnterAndConjunction is called when entering the AndConjunction production.
func (z *ToSquirrelListener) EnterAndConjunction(c *zitiql.AndConjunctionContext) {
	z.printDebug(c)
	z.parseState.AddAnd()
}

// EnterInStringArrayOp is called when entering the InStringArrayOp production.
func (z *ToSquirrelListener) EnterInStringArrayOp(c *zitiql.InStringArrayOpContext) {
	z.printDebug(c)

	var identifier string
	var values []string
	var opText string

	for _, c := range c.GetChildren() {

		if tn, ok := c.(antlr.TerminalNode); ok {
			tt := tn.GetSymbol().GetTokenType()
			switch tt {
			case zitiql.ZitiQlLexerIDENTIFIER:
				identifier = z.translateIdentifier(&tn)
			case zitiql.ZitiQlLexerIN:
				opText = tn.GetText()
			}

		} else {
			for _, ac := range c.GetChildren() {
				if atn, ok := ac.(antlr.TerminalNode); ok {
					if atn.GetSymbol().GetTokenType() == zitiql.ZitiQlLexerSTRING {
						value := parseString(atn.GetText())
						values = append(values, value)
					}
				}
			}
		}
	}

	if caseInsensitiveContains(opText, "not") {
		z.parseState.AddOp(newOp(squirrel.NotEq{identifier: values}, identifier, NotInOp, values, StringArrayType))
	} else {
		z.parseState.AddOp(newOp(squirrel.Eq{identifier: values}, identifier, InOp, values, StringArrayType))
	}
}

// EnterInNumberArrayOp is called when entering the InNumberArrayOp production.
func (z *ToSquirrelListener) EnterInNumberArrayOp(c *zitiql.InNumberArrayOpContext) {
	z.printDebug(c)

	var identifier string
	var values []interface{}
	var opText string

	for _, c := range c.GetChildren() {

		if tn, ok := c.(antlr.TerminalNode); ok {
			tt := tn.GetSymbol().GetTokenType()
			switch tt {
			case zitiql.ZitiQlLexerIDENTIFIER:
				identifier = z.translateIdentifier(&tn)
			case zitiql.ZitiQlLexerIN:
				opText = tn.GetText()
			}
		} else {
			for _, ac := range c.GetChildren() {
				if atn, ok := ac.(antlr.TerminalNode); ok {
					if atn.GetSymbol().GetTokenType() == zitiql.ZitiQlLexerNUMBER {
						value, err := parseNumber(atn.GetText())

						if err != nil {
							z.HandleError(err)
						}

						values = append(values, value)
					}
				}
			}
		}
	}

	if caseInsensitiveContains(opText, "not") {
		z.parseState.AddOp(newOp(squirrel.NotEq{identifier: values}, identifier, NotInOp, values, NumberArrayType))
	} else {
		z.parseState.AddOp(newOp(squirrel.Eq{identifier: values}, identifier, InOp, values, NumberArrayType))
	}

}

// EnterInDatetimeArrayOp is called when entering the InDatetimeArrayOp production.
func (z *ToSquirrelListener) EnterInDatetimeArrayOp(c *zitiql.InDatetimeArrayOpContext) {
	z.printDebug(c)

	var identifier string
	var values []time.Time
	var opText string

	for _, c := range c.GetChildren() {

		if tn, ok := c.(antlr.TerminalNode); ok {
			tt := tn.GetSymbol().GetTokenType()
			switch tt {
			case zitiql.ZitiQlLexerIDENTIFIER:
				identifier = z.translateIdentifier(&tn)
			case zitiql.ZitiQlLexerIN:
				opText = tn.GetText()
			}

		} else {
			for _, ac := range c.GetChildren() {
				if atn, ok := ac.(antlr.TerminalNode); ok {
					if atn.GetSymbol().GetTokenType() == zitiql.ZitiQlLexerDATETIME {
						value, err := parseDatetime(atn.GetText())

						if err != nil {
							z.HandleError(err)
						}

						values = append(values, value)
					}
				}
			}
		}
	}
	if caseInsensitiveContains(opText, "not") {
		z.parseState.AddOp(newOp(squirrel.NotEq{identifier: values}, identifier, NotInOp, values, DatetimeArrayType))
	} else {
		z.parseState.AddOp(newOp(squirrel.Eq{identifier: values}, identifier, InOp, values, DatetimeArrayType))
	}

}

// EnterBetweenNumberOp is called when entering the BetweenNumberOp production.
func (z *ToSquirrelListener) EnterBetweenNumberOp(c *zitiql.BetweenNumberOpContext) {
	z.printDebug(c)

	var identifier string
	var text1 string
	var text2 string
	var opText string

	for _, c := range c.GetChildren() {
		tn := c.(antlr.TerminalNode)
		if tn != nil {
			tt := tn.GetSymbol().GetTokenType()
			switch tt {
			case zitiql.ZitiQlLexerIDENTIFIER:
				identifier = z.translateIdentifier(&tn)
			case zitiql.ZitiQlLexerNUMBER:
				if text1 == "" {
					text1 = tn.GetText()
				} else {
					text2 = tn.GetText()
				}
			case zitiql.ZitiQlLexerBETWEEN:
				opText = tn.GetText()
			}
		}
	}

	value1, err := parseNumber(text1)

	if err != nil {
		z.HandleError(err)
	}

	value2, err := parseNumber(text2)

	if err != nil {
		z.HandleError(err)
	}

	if caseInsensitiveContains(opText, "not") {
		z.parseState.AddOp(newOp(newNotBetween(identifier, value1, value2), identifier, NotBetweenOp, []interface{}{value1, value2}, NumberType))
	} else {
		z.parseState.AddOp(newOp(newBetween(identifier, value1, value2), identifier, BetweenOp, []interface{}{value1, value2}, NumberType))
	}

}

// EnterBetweenDateOp is called when entering the BetweenDateOp production.
func (z *ToSquirrelListener) EnterBetweenDateOp(c *zitiql.BetweenDateOpContext) {
	z.printDebug(c)

	var identifier string
	var text1 string
	var text2 string
	var opText string

	for _, c := range c.GetChildren() {
		tn := c.(antlr.TerminalNode)
		if tn != nil {
			tt := tn.GetSymbol().GetTokenType()
			switch tt {
			case zitiql.ZitiQlLexerIDENTIFIER:
				identifier = z.translateIdentifier(&tn)
			case zitiql.ZitiQlLexerDATETIME:
				if text1 == "" {
					text1 = tn.GetText()
				} else {
					text2 = tn.GetText()
				}
			case zitiql.ZitiQlLexerBETWEEN:
				opText = tn.GetText()
			}
		}
	}

	value1, err := parseDatetime(text1)

	if err != nil {
		z.HandleError(err)
	}

	value2, err := parseDatetime(text2)

	if err != nil {
		z.HandleError(err)
	}

	if caseInsensitiveContains(opText, "not") {
		z.parseState.AddOp(newOp(newNotBetween(identifier, value1, value2), identifier, NotBetweenOp, []interface{}{value1, value2}, DatetimeType))
	} else {
		z.parseState.AddOp(newOp(newBetween(identifier, value1, value2), identifier, BetweenOp, []interface{}{value1, value2}, DatetimeType))
	}

}

// EnterBinaryLessThanNumberOp is called when entering the BinaryLessThanNumberOp production.
func (z *ToSquirrelListener) EnterBinaryLessThanNumberOp(c *zitiql.BinaryLessThanNumberOpContext) {
	z.printDebug(c)

	var identifier string
	var text string
	var opText string

	for _, c := range c.GetChildren() {
		tn := c.(antlr.TerminalNode)
		if tn != nil {
			switch tn.GetSymbol().GetTokenType() {
			case zitiql.ZitiQlLexerIDENTIFIER:
				identifier = z.translateIdentifier(&tn)
			case zitiql.ZitiQlLexerNUMBER:
				text = tn.GetText()
			case zitiql.ZitiQlLexerLT:
				opText = tn.GetText()
			}
		}
	}

	value, err := parseNumber(text)

	if err != nil {
		z.HandleError(err)
		return
	}

	if opText == "<" {
		z.parseState.AddOp(newOp(squirrel.Lt{identifier: value}, identifier, LtOp, value, NumberType))
	} else {
		z.parseState.AddOp(newOp(squirrel.LtOrEq{identifier: value}, identifier, LtEOp, value, NumberType))
	}
}

// EnterBinaryLessThanDatetimeOp is called when entering the BinaryLessThanDatetimeOp production.
func (z *ToSquirrelListener) EnterBinaryLessThanDatetimeOp(c *zitiql.BinaryLessThanDatetimeOpContext) {
	z.printDebug(c)

	var identifier string
	var text string
	var opText string

	for _, c := range c.GetChildren() {
		tn := c.(antlr.TerminalNode)
		if tn != nil {
			tt := tn.GetSymbol().GetTokenType()
			switch tt {
			case zitiql.ZitiQlLexerIDENTIFIER:
				identifier = z.translateIdentifier(&tn)
			case zitiql.ZitiQlLexerDATETIME:
				text = tn.GetText()
			case zitiql.ZitiQlLexerLT:
				opText = tn.GetText()
			}
		}
	}

	value, err := parseDatetime(text)

	if err != nil {
		z.HandleError(err)
		return
	}

	if opText == "<=" {
		z.parseState.AddOp(newOp(squirrel.LtOrEq{identifier: value}, identifier, LtEOp, value, DatetimeType))
	} else {
		z.parseState.AddOp(newOp(squirrel.Lt{identifier: value}, identifier, LtOp, value, DatetimeType))
	}
}

// EnterBinaryGreaterThanNumberOp is called when entering the BinaryGreaterThanNumberOp production.
func (z *ToSquirrelListener) EnterBinaryGreaterThanNumberOp(c *zitiql.BinaryGreaterThanNumberOpContext) {
	z.printDebug(c)

	var identifier string
	var text string
	var opText string

	for _, c := range c.GetChildren() {
		tn := c.(antlr.TerminalNode)
		if tn != nil {
			switch tn.GetSymbol().GetTokenType() {
			case zitiql.ZitiQlLexerIDENTIFIER:
				identifier = z.translateIdentifier(&tn)
			case zitiql.ZitiQlLexerNUMBER:
				text = tn.GetText()
			case zitiql.ZitiQlLexerGT:
				opText = tn.GetText()
			}
		}
	}
	value, err := parseNumber(text)

	if err != nil {
		z.HandleError(err)
		return
	}

	if opText == ">" {
		z.parseState.AddOp(newOp(squirrel.Gt{identifier: value}, identifier, GtOp, value, NumberType))
	} else {
		z.parseState.AddOp(newOp(squirrel.GtOrEq{identifier: value}, identifier, GtEOp, value, NumberType))
	}
}

// EnterBinaryGreaterThanDatetimeOp is called when entering the BinaryGreaterThanDatetimeOp production.
func (z *ToSquirrelListener) EnterBinaryGreaterThanDatetimeOp(c *zitiql.BinaryGreaterThanDatetimeOpContext) {
	z.printDebug(c)

	var identifier string
	var text string
	var opText string

	for _, c := range c.GetChildren() {
		tn := c.(antlr.TerminalNode)
		if tn != nil {
			tt := tn.GetSymbol().GetTokenType()
			switch tt {
			case zitiql.ZitiQlLexerIDENTIFIER:
				identifier = z.translateIdentifier(&tn)
			case zitiql.ZitiQlLexerDATETIME:
				text = tn.GetText()
			case zitiql.ZitiQlLexerGT:
				opText = tn.GetText()
			}
		}
	}

	value, err := parseDatetime(text)

	if err != nil {
		z.HandleError(err)
		return
	}

	if opText == ">=" {
		z.parseState.AddOp(newOp(squirrel.GtOrEq{identifier: value}, identifier, GtEOp, value, DatetimeType))
	} else {
		z.parseState.AddOp(newOp(squirrel.Gt{identifier: value}, identifier, GtOp, value, DatetimeType))
	}
}

// EnterBinaryEqualToStringOp is called when entering the BinaryEqualToStringOp production.
func (z *ToSquirrelListener) EnterBinaryEqualToStringOp(c *zitiql.BinaryEqualToStringOpContext) {
	z.printDebug(c)

	var identifier string
	var text string
	var opText string

	for _, c := range c.GetChildren() {
		tn := c.(antlr.TerminalNode)
		if tn != nil {
			tt := tn.GetSymbol().GetTokenType()
			switch tt {
			case zitiql.ZitiQlLexerIDENTIFIER:
				identifier = z.translateIdentifier(&tn)
			case zitiql.ZitiQlLexerSTRING:
				text = parseString(tn.GetText())
			case zitiql.ZitiQlLexerEQ:
				opText = tn.GetText()
			}
		}
	}

	if opText == "!=" {
		z.parseState.AddOp(newOp(squirrel.NotEq{identifier: text}, identifier, NeqOp, text, DatetimeType))
	} else {
		z.parseState.AddOp(newOp(squirrel.Eq{identifier: text}, identifier, EqOp, text, DatetimeType))
	}
}

// EnterBinaryEqualToNumberOp is called when entering the BinaryEqualToNumberOp production.
func (z *ToSquirrelListener) EnterBinaryEqualToNumberOp(c *zitiql.BinaryEqualToNumberOpContext) {
	z.printDebug(c)

	var identifier string
	var text string
	var opText string

	for _, c := range c.GetChildren() {
		tn := c.(antlr.TerminalNode)
		if tn != nil {
			switch tn.GetSymbol().GetTokenType() {
			case zitiql.ZitiQlLexerIDENTIFIER:
				identifier = z.translateIdentifier(&tn)
			case zitiql.ZitiQlLexerNUMBER:
				text = tn.GetText()
			case zitiql.ZitiQlLexerEQ:
				opText = tn.GetText()
			}
		}
	}

	value, err := parseNumber(text)

	if err != nil {
		z.HandleError(err)
		return
	}

	if opText == "!=" {
		z.parseState.AddOp(newOp(squirrel.NotEq{identifier: value}, identifier, NeqOp, value, NumberType))
	} else {
		z.parseState.AddOp(newOp(squirrel.Eq{identifier: value}, identifier, EqOp, value, NumberType))
	}
}

// EnterBinaryEqualToDatetimeOp is called when entering the BinaryEqualToDatetimeOp production.
func (z *ToSquirrelListener) EnterBinaryEqualToDatetimeOp(c *zitiql.BinaryEqualToDatetimeOpContext) {
	z.printDebug(c)

	var identifier string
	var text string
	var opText string

	for _, c := range c.GetChildren() {
		tn := c.(antlr.TerminalNode)
		if tn != nil {
			tt := tn.GetSymbol().GetTokenType()
			switch tt {
			case zitiql.ZitiQlLexerIDENTIFIER:
				identifier = z.translateIdentifier(&tn)
			case zitiql.ZitiQlLexerDATETIME:
				text = tn.GetText()
			case zitiql.ZitiQlLexerEQ:
				opText = tn.GetText()
			}
		}
	}

	value, err := parseDatetime(text)

	if err != nil {
		z.HandleError(err)
	}

	if opText == "!=" {
		z.parseState.AddOp(newOp(squirrel.NotEq{identifier: value}, identifier, NeqOp, value, DatetimeType))
	} else {
		z.parseState.AddOp(newOp(squirrel.Eq{identifier: value}, identifier, EqOp, value, DatetimeType))
	}
}

// EnterBinaryEqualToBoolOp is called when entering the BinaryEqualToBoolOp production.
func (z *ToSquirrelListener) EnterBinaryEqualToBoolOp(c *zitiql.BinaryEqualToBoolOpContext) {
	z.printDebug(c)

	var identifier string
	var text string
	var opText string

	for _, c := range c.GetChildren() {
		tn := c.(antlr.TerminalNode)
		if tn != nil {
			switch tn.GetSymbol().GetTokenType() {
			case zitiql.ZitiQlLexerIDENTIFIER:
				identifier = z.translateIdentifier(&tn)
			case zitiql.ZitiQlLexerBOOL:
				text = tn.GetText()
			case zitiql.ZitiQlLexerEQ:
				opText = tn.GetText()
			}
		}
	}

	value, err := strconv.ParseBool(strings.ToLower(text))

	if err != nil {
		z.HandleError(err)
		return
	}

	if opText == "!=" {
		z.parseState.AddOp(newOp(squirrel.NotEq{identifier: value}, identifier, NeqOp, value, BoolType))
	} else {
		z.parseState.AddOp(newOp(squirrel.Eq{identifier: value}, identifier, EqOp, value, BoolType))
	}
}

// EnterBinaryEqualToNullOp is called when entering the BinaryEqualToNullOp production.
func (z *ToSquirrelListener) EnterBinaryEqualToNullOp(c *zitiql.BinaryEqualToNullOpContext) {
	z.printDebug(c)

	var identifier string
	var opText string

	for _, c := range c.GetChildren() {
		tn := c.(antlr.TerminalNode)
		if tn != nil {
			switch tn.GetSymbol().GetTokenType() {
			case zitiql.ZitiQlLexerIDENTIFIER:
				identifier = z.translateIdentifier(&tn)
			case zitiql.ZitiQlLexerEQ:
				opText = tn.GetText()
			}
		}
	}

	if opText == "!=" {
		z.parseState.AddOp(newOp(squirrel.NotEq{identifier: nil}, identifier, NeqOp, nil, NullType))
	} else {
		z.parseState.AddOp(newOp(squirrel.Eq{identifier: nil}, identifier, EqOp, nil, NullType))
	}
}

// EnterBinaryContainsOp is called when entering the BinaryContainsOp production.
func (z *ToSquirrelListener) EnterBinaryContainsOp(c *zitiql.BinaryContainsOpContext) {
	z.printDebug(c)

	var identifier string
	var text string
	var opText string

	for _, c := range c.GetChildren() {
		tn := c.(antlr.TerminalNode)
		if tn != nil {
			switch tn.GetSymbol().GetTokenType() {
			case zitiql.ZitiQlLexerIDENTIFIER:
				identifier = z.translateIdentifier(&tn)
			case zitiql.ZitiQlLexerSTRING:
				text = parseString(tn.GetText())
			case zitiql.ZitiQlLexerCONTAINS:
				opText = strings.ToLower(tn.GetText())
			}
		}
	}

	if strings.Contains(opText, "not") {
		z.parseState.AddOp(newOp(newNotContains(identifier, text), identifier, ContainsOp, text, StringType))
	} else {
		z.parseState.AddOp(newOp(newContains(identifier, text), identifier, ContainsOp, text, StringType))
	}
}

// ExitEnd is called when exiting the End production.
func (z *ToSquirrelListener) ExitEnd(c *zitiql.EndContext) {
	z.printDebug(c)
	z.Predicate, z.IdentifierOps = z.parseState.End()
}

// ExitGroup is called when exiting the Group production.
func (z *ToSquirrelListener) ExitGroup(c *zitiql.GroupContext) {
	z.printDebug(c)
	z.parseState.ExitGroup()
}

// ExitOrConjunction is called when exiting the OrConjunction production.
func (z *ToSquirrelListener) ExitOrConjunction(c *zitiql.OrConjunctionContext) { z.printDebug(c) }

// ExitOperationOp is called when exiting the OperationOp production.
func (z *ToSquirrelListener) ExitOperationOp(c *zitiql.OperationOpContext) { z.printDebug(c) }

// ExitAndConjunction is called when exiting the AndConjunction production.
func (z *ToSquirrelListener) ExitAndConjunction(c *zitiql.AndConjunctionContext) { z.printDebug(c) }

// ExitInStringArrayOp is called when exiting the InStringArrayOp production.
func (z *ToSquirrelListener) ExitInStringArrayOp(c *zitiql.InStringArrayOpContext) { z.printDebug(c) }

// ExitInNumberArrayOp is called when exiting the InNumberArrayOp production.
func (z *ToSquirrelListener) ExitInNumberArrayOp(c *zitiql.InNumberArrayOpContext) { z.printDebug(c) }

// ExitInDatetimeArrayOp is called when exiting the InDatetimeArrayOp production.
func (z *ToSquirrelListener) ExitInDatetimeArrayOp(c *zitiql.InDatetimeArrayOpContext) {
	z.printDebug(c)
}

// ExitBetweenNumberOp is called when exiting the BetweenNumberOp production.
func (z *ToSquirrelListener) ExitBetweenNumberOp(c *zitiql.BetweenNumberOpContext) { z.printDebug(c) }

// ExitBetweenDateOp is called when exiting the BetweenDateOp production.
func (z *ToSquirrelListener) ExitBetweenDateOp(c *zitiql.BetweenDateOpContext) { z.printDebug(c) }

// ExitBinaryLessThanNumberOp is called when exiting the BinaryLessThanNumberOp production.
func (z *ToSquirrelListener) ExitBinaryLessThanNumberOp(c *zitiql.BinaryLessThanNumberOpContext) {
	z.printDebug(c)
}

// ExitBinaryLessThanDatetimeOp is called when exiting the BinaryLessThanDatetimeOp production.
func (z *ToSquirrelListener) ExitBinaryLessThanDatetimeOp(c *zitiql.BinaryLessThanDatetimeOpContext) {
	z.printDebug(c)
}

// ExitBinaryGreaterThanNumberOp is called when exiting the BinaryGreaterThanNumberOp production.
func (z *ToSquirrelListener) ExitBinaryGreaterThanNumberOp(c *zitiql.BinaryGreaterThanNumberOpContext) {
	z.printDebug(c)
}

// ExitBinaryGreaterThanDatetimeOp is called when exiting the BinaryGreaterThanDatetimeOp production.
func (z *ToSquirrelListener) ExitBinaryGreaterThanDatetimeOp(c *zitiql.BinaryGreaterThanDatetimeOpContext) {
	z.printDebug(c)
}

// ExitBinaryEqualToStringOp is called when exiting the BinaryEqualToStringOp production.
func (z *ToSquirrelListener) ExitBinaryEqualToStringOp(c *zitiql.BinaryEqualToStringOpContext) {
	z.printDebug(c)
}

// ExitBinaryEqualToNumberOp is called when exiting the BinaryEqualToNumberOp production.
func (z *ToSquirrelListener) ExitBinaryEqualToNumberOp(c *zitiql.BinaryEqualToNumberOpContext) {
	z.printDebug(c)
}

// ExitBinaryEqualToDatetimeOp is called when exiting the BinaryEqualToDatetimeOp production.
func (z *ToSquirrelListener) ExitBinaryEqualToDatetimeOp(c *zitiql.BinaryEqualToDatetimeOpContext) {
	z.printDebug(c)
}

// ExitBinaryEqualToBoolOp is called when exiting the BinaryEqualToBoolOp production.
func (z *ToSquirrelListener) ExitBinaryEqualToBoolOp(c *zitiql.BinaryEqualToBoolOpContext) {
	z.printDebug(c)
}

// ExitBinaryEqualToNullOp is called when exiting the BinaryEqualToNullOp production.
func (z *ToSquirrelListener) ExitBinaryEqualToNullOp(c *zitiql.BinaryEqualToNullOpContext) {
	z.printDebug(c)
}

// ExitBinaryContainsOp is called when exiting the BinaryContainsOp production.
func (z *ToSquirrelListener) ExitBinaryContainsOp(c *zitiql.BinaryContainsOpContext) { z.printDebug(c) }

func parseNumber(text string) (interface{}, error) {
	value, intErr := strconv.Atoi(text)

	if intErr != nil {
		value, floatErr := strconv.ParseFloat(text, 64)

		if floatErr != nil {
			return 0, fmt.Errorf("could not parse number as float or int: int(%s) and float(%s)", intErr, floatErr)
		} else {
			return value, nil
		}

	} else {
		return value, nil
	}
}

var dateTimeStripper = regexp.MustCompile(`^\s*datetime\(\s*(.*?)\s*\)\s*$`)

func parseDatetime(text string) (time.Time, error) {
	m := dateTimeStripper.FindAllStringSubmatch(text, -1)

	if m == nil || len(m) != 1 || len(m[0]) != 2 {
		return time.Time{}, fmt.Errorf("could not parse datetime (%s)", text)
	}

	//RFC3339 allows 'z','Z','t', and 'T'. Go's implementation only allows 'Z' and 'T'
	s := strings.Replace(m[0][1], "z", "Z", 1)
	s = strings.Replace(s, "t", "T", 1)

	t, err := time.Parse(time.RFC3339, s)

	return t, err
}

func parseString(text string) string {
	t := strings.TrimSuffix(strings.TrimPrefix(text, `"`), `"`)

	//remove golang string back slash escaping
	t = strings.Replace(t, `\\`, `\`, -1)

	//remove ZitiQL string escaping
	t = strings.Replace(t, `\"`, `"`, -1)
	t = strings.Replace(t, `\f`, "\f", -1)
	t = strings.Replace(t, `\n`, "\n", -1)
	t = strings.Replace(t, `\r`, "\r", -1)
	t = strings.Replace(t, `\t`, "\t", -1)
	t = strings.Replace(t, `\\`, `\`, -1)

	return t
}
