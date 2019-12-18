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
	"github.com/jinzhu/gorm"
	"github.com/netfoundry/ziti-edge/controller/zitiql"
	"gopkg.in/Masterminds/squirrel.v1"
	"regexp"
	"strings"
)

type Direction string

const (
	ASC  Direction = "ASC"
	DESC Direction = "DESC"
)

type IdentifierMap map[string]string

func (im *IdentifierMap) Fields() []string {
	var ret []string

	for k := range *im {
		ret = append(ret, k)
	}

	return ret
}

type Predicate struct {
	Clause   squirrel.Sqlizer
	Callback func(db *gorm.DB) *gorm.DB
}

func (p *Predicate) Apply(q *gorm.DB) *gorm.DB {
	if p.Callback != nil {
		q = p.Callback(q)
	}

	if p.Clause != nil {
		clause, args, _ := p.Clause.ToSql()
		q = q.Where(clause, args...)
	}

	return q
}

type Paging struct {
	Offset    int64
	Limit     int64
	ReturnAll bool
}

func (paging *Paging) String() string {
	if paging == nil {
		return "nil"
	}
	return fmt.Sprintf("[Paging Offset: '%v', Limit: '%v', ReturnAll: '%v']", paging.Offset, paging.Limit, paging.ReturnAll)
}

type Sort []SortField

type SortField struct {
	Field     string
	Direction Direction
}

func (sf *SortField) String() string {
	return fmt.Sprintf("%s %s", sf.Field, string(sf.Direction))
}

func (s *Sort) Parts() []string {
	var ret []string

	for _, f := range *s {
		ret = append(ret, f.String())
	}

	return ret
}

type ParseError struct {
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Symbol  string `json:"symbol"`
	Message string `json:"message"`
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s at line %d, column %d near %s", e.Message, e.Line, e.Column, e.Symbol)
}

func NewParseError(pe *zitiql.ParseError) *ParseError {
	return &ParseError{
		Symbol:  pe.Symbol,
		Column:  pe.Column,
		Line:    pe.Line,
		Message: pe.Message,
	}
}

func ParseWhereClause(str string, imap *IdentifierMap) (squirrel.Sqlizer, []error) {
	listener := NewSquirrelListenerWithMap(imap)
	pe := zitiql.Parse(str, listener)
	var errs []error

	for _, ze := range append(pe, listener.IdentifierErrors...) {
		errs = append(errs, NewParseError(&ze))
	}

	return listener.Predicate, errs
}

var orderByTokenizer = regexp.MustCompile(`^\s*([a-zA-Z]+[\.a-zA-Z0-9]*)\s+(ASC|DESC)\s*(,|$)`)

func ParseOrderBy(str string, imap *IdentifierMap) (*Sort, error) {
	column := 0

	sort := Sort{}

	for strings.TrimSpace(str) != "" {
		matches := orderByTokenizer.FindStringSubmatch(str)
		if matches == nil {
			pe := &ParseError{
				Line:    1,
				Column:  column,
				Symbol:  str,
				Message: "Unexpected symbol",
			}
			return nil, pe
		}
		if id, ok := (*imap)[matches[1]]; ok {
			field := SortField{
				Field:     id,
				Direction: Direction(matches[2]),
			}

			sort = append(sort, field)
		} else {

			ie := &ParseError{
				Column:  column,
				Line:    1,
				Symbol:  matches[1],
				Message: "Invalid identifier",
			}
			return nil, ie
		}

		column += len(matches[0])
		str = strings.TrimSpace(strings.Replace(str, matches[0], "", 1))
	}

	return &sort, nil
}
