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

package zitiql

import (
	"fmt"
	"github.com/antlr/antlr4/runtime/Go/antlr"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	lexerPool = &sync.Pool{New: func() interface{} {
		return NewZitiQlLexer(nil)
	}}

	parserPool = &sync.Pool{New: func() interface{} {
		return NewZitiQlParser(nil)
	}}
)

func parse(str string, l ZitiQlListener, el antlr.ErrorListener, debug bool) {
	lexer := lexerPool.Get().(*ZitiQlLexer)
	defer lexerPool.Put(lexer)

	input := antlr.NewInputStream(str)
	lexer.SetInputStream(input)

	p := parserPool.Get().(*ZitiQlParser)
	defer parserPool.Put(p)

	stream := antlr.NewCommonTokenStream(lexer, 0)
	p.SetInputStream(stream)

	if debug {
		p.AddErrorListener(antlr.NewDiagnosticErrorListener(true))
	} else {
		p.RemoveErrorListeners()
	}

	p.AddErrorListener(el)

	p.BuildParseTrees = true
	tree := p.Start()

	antlr.ParseTreeWalkerDefault.Walk(l, tree)
}

func Parse(str string, l ZitiQlListener) []ParseError {
	return ParseWithDebug(str, l, false)
}

func ParseWithDebug(str string, l ZitiQlListener, debug bool) []ParseError {
	el := newErrorListener()
	parse(str, l, el, debug)
	return el.Errors
}

type ParseError struct {
	Line    int
	Column  int
	Symbol  string
	Message string
}

func (p ParseError) Error() string {
	return fmt.Sprintf("%v. line: %v, column: %v, symbol: %v", p.Message, p.Line, p.Column, p.Symbol)
}

func newErrorListener() *ErrorListener {
	return &ErrorListener{
		Errors: []ParseError{},
	}
}

type ErrorListener struct {
	Errors []ParseError
}

func (el *ErrorListener) SyntaxError(_ antlr.Recognizer, offendingSymbol interface{}, line, column int, _ string, _ antlr.RecognitionException) {
	s, ok := offendingSymbol.(*antlr.CommonToken)
	symbol := "<unknown>"
	if ok {
		symbol = s.GetText()
	}

	el.Errors = append(el.Errors, ParseError{
		Line:    line,
		Column:  column,
		Symbol:  symbol,
		Message: fmt.Sprintf(`Unexpected symbol: "%s" at line: %d column: %d`, s.GetText(), line, column),
	})
}

func (el *ErrorListener) ReportAmbiguity(antlr.Parser, *antlr.DFA, int, int, bool, *antlr.BitSet, antlr.ATNConfigSet) {
	// ignored
}

func (el *ErrorListener) ReportAttemptingFullContext(antlr.Parser, *antlr.DFA, int, int, *antlr.BitSet, antlr.ATNConfigSet) {
	// ignored
}

func (el *ErrorListener) ReportContextSensitivity(antlr.Parser, *antlr.DFA, int, int, int, antlr.ATNConfigSet) {
	// ignored
}

func ParseZqlString(text string) string {
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

var dateTimeStripper = regexp.MustCompile(`^\s*datetime\(\s*(.*?)\s*\)\s*$`)

func ParseZqlDatetime(text string) (time.Time, error) {
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
