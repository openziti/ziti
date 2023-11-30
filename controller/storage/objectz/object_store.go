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

package objectz

import (
	"fmt"
	"github.com/biogo/store/llrb"
	"github.com/openziti/storage/ast"
	"math"
	"time"
)

type ObjectIterator[T any] interface {
	IsValid() bool
	Next()
	Current() T
}

func NewObjectStore[T any](iteratorF func() ObjectIterator[T]) *ObjectStore[T] {
	return &ObjectStore[T]{
		symbols:   map[string]ObjectSymbol[T]{},
		iteratorF: iteratorF,
	}
}

type ObjectStore[T any] struct {
	symbols   map[string]ObjectSymbol[T]
	iteratorF func() ObjectIterator[T]
}

func (self *ObjectStore[T]) AddBoolSymbol(name string, f func(entity T) *bool) {
	self.symbols[name] = &ObjectBoolSymbol[T]{
		f: f,
	}
}

func (self *ObjectStore[T]) AddStringSymbol(name string, f func(entity T) *string) {
	self.symbols[name] = &ObjectStringSymbol[T]{
		f: f,
	}
}

func (self *ObjectStore[T]) AddInt64Symbol(name string, f func(entity T) *int64) {
	self.symbols[name] = &ObjectInt64Symbol[T]{
		f: f,
	}
}

func (self *ObjectStore[T]) AddFloat64Symbol(name string, f func(entity T) *float64) {
	self.symbols[name] = &ObjectFloat64Symbol[T]{
		f: f,
	}
}

func (self *ObjectStore[T]) AddDatetimeSymbol(name string, f func(entity T) *time.Time) {
	self.symbols[name] = &ObjectDatetimeSymbol[T]{
		f: f,
	}
}

func (self *ObjectStore[T]) GetSymbol(name string) ObjectSymbol[T] {
	return self.symbols[name]
}

func (self *ObjectStore[T]) GetSymbolType(name string) (ast.NodeType, bool) {
	if symbol := self.GetSymbol(name); symbol != nil {
		return symbol.GetType(), true
	}
	return 0, false
}

func (self *ObjectStore[T]) GetSetSymbolTypes(name string) ast.SymbolTypes {
	//if symbol := self.GetSymbol(name); symbol != nil {
	//	return symbol.GetLinkedType()
	//}
	return nil
}

func (self *ObjectStore[T]) IsSet(name string) (bool, bool) {
	//if symbol := self.GetSymbol(name); symbol != nil {
	//	return symbol.IsSet(), true
	//}
	return false, true
}

func (store *ObjectStore[T]) newRowComparator(sort []ast.SortField) (objectComparator[T], error) {
	// always have id as last sort element. this way if other sorts come out equal, we still
	// can order on something, instead of having duplicates which causes rows to get discarded
	sort = append(sort, ast.NewSortFieldNode("id", true))

	var symbolsComparators []objectComparator[T]
	for _, sortField := range sort {
		symbol, found := store.symbols[sortField.Symbol()]
		forward := sortField.IsAscending()
		if !found {
			return nil, fmt.Errorf("no such sort field: %v", sortField.Symbol())
		}

		//if symbol.IsSet() {
		//	return nil, fmt.Errorf("invalid sort field: %v", sortField.Symbol())
		//}

		var comparator objectComparator[T]
		switch symbol.GetType() {
		case ast.NodeTypeBool:
			comparator = &objectBoolSymbolComparator[T]{
				symbol:  symbol.(*ObjectBoolSymbol[T]),
				forward: forward,
			}
		case ast.NodeTypeDatetime:
			comparator = &objectDatetimeSymbolComparator[T]{
				symbol:  symbol.(*ObjectDatetimeSymbol[T]),
				forward: forward,
			}
		case ast.NodeTypeFloat64:
			comparator = &objectFloat64SymbolComparator[T]{
				symbol:  symbol.(*ObjectFloat64Symbol[T]),
				forward: forward,
			}
		case ast.NodeTypeInt64:
			comparator = &objectInt64SymbolComparator[T]{
				symbol:  symbol.(*ObjectInt64Symbol[T]),
				forward: forward,
			}
		case ast.NodeTypeString:
			comparator = &objectStringSymbolComparator[T]{
				symbol:  symbol.(*ObjectStringSymbol[T]),
				forward: forward,
			}
		default:
			return nil, fmt.Errorf("unsupported sort field type %v for field : %v", ast.NodeTypeName(symbol.GetType()), sortField.Symbol())
		}
		symbolsComparators = append(symbolsComparators, comparator)
	}

	return &compoundObjectComparator[T]{comparators: symbolsComparators}, nil
}

func (self *ObjectStore[T]) QueryEntities(queryString string) ([]T, int64, error) {
	query, err := ast.Parse(self, queryString)
	if err != nil {
		return nil, 0, err
	}
	return self.QueryEntitiesC(query)
}

func (self *ObjectStore[T]) QueryEntitiesC(query ast.Query) ([]T, int64, error) {
	s := &memSortingScanner[T]{}
	return s.Scan(self, query)
}

type scanner struct {
	targetOffset int64
	targetLimit  int64
}

func (s *scanner) setPaging(query ast.Query) {
	if query.GetSkip() == nil {
		query.SetSkip(0)
	}
	s.targetOffset = *query.GetSkip()

	if query.GetLimit() == nil || *query.GetLimit() < 0 {
		query.SetLimit(math.MaxInt64)
	}

	s.targetLimit = *query.GetLimit()
}

type memSortingScanner[T any] struct {
	scanner
	offset int64
	count  int64
}

func (scanner *memSortingScanner[T]) Scan(store *ObjectStore[T], query ast.Query) ([]T, int64, error) {
	scanner.setPaging(query)
	comparator, err := store.newRowComparator(query.GetSortFields())
	if err != nil {
		return nil, 0, err
	}

	cursor := store.iteratorF()

	rowCursor := &ObjectCursor[T]{
		store:   store,
		current: cursor.Current(),
	}

	if cursor == nil {
		return nil, 0, nil
	}

	// Longer term, if we're looking for better performance, we could make a version of llrb which takes a comparator
	// function instead of putting the comparison on the elements, so we don't need to store a context with each row
	results := &llrb.Tree{}
	maxResults := scanner.targetOffset + scanner.targetLimit
	for cursor.IsValid() {
		rowCursor.current = cursor.Current()
		cursor.Next()
		if query.EvalBool(rowCursor) {
			results.Insert(&memEntityComparable[T]{
				comparator: comparator,
				entity:     rowCursor.current,
			})
			scanner.count++
			if scanner.count > maxResults {
				results.DeleteMax()
			}
		}
	}

	var result []T
	results.Do(func(row llrb.Comparable) bool {
		if scanner.offset < scanner.targetOffset {
			scanner.offset++
		} else {
			result = append(result, row.(*memEntityComparable[T]).entity)
		}
		return false
	})

	return result, scanner.count, nil
}

type memEntityComparable[T any] struct {
	comparator objectComparator[T]
	entity     T
}

func (self memEntityComparable[T]) Compare(c llrb.Comparable) int {
	other := c.(*memEntityComparable[T])
	return self.comparator.compare(self.entity, other.entity)
}

func IterateMap[T any](m map[string]T) ObjectIterator[T] {
	c := make(chan T, 1)
	go func() {
		for _, v := range m {
			c <- v
		}
		close(c)
	}()
	iterator := &ChannelIterator[T]{
		c:     c,
		valid: true,
	}
	iterator.Next()
	return iterator
}

type ChannelIterator[T any] struct {
	c       <-chan T
	current T
	valid   bool
}

func (self *ChannelIterator[T]) IsValid() bool {
	return self.valid
}

func (self *ChannelIterator[T]) Next() {
	next, ok := <-self.c
	if !ok {
		self.valid = false
	} else {
		self.current = next
	}
}

func (self *ChannelIterator[T]) Current() T {
	return self.current
}
