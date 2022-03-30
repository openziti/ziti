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

package boltz

import (
	"math"

	"github.com/biogo/store/llrb"
	"github.com/openziti/storage/ast"
	"go.etcd.io/bbolt"
)

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

type uniqueIndexScanner struct {
	scanner
	store     ListStore
	forward   bool
	offset    int64
	count     int64
	collected int64

	cursor    ast.SetCursor
	rowCursor *rowCursorImpl
	filter    ast.BoolNode
	current   []byte
}

func newCursorScanner(tx *bbolt.Tx, store ListStore, cursor ast.SetCursor, query ast.Query) ast.SetCursor {
	result := &uniqueIndexScanner{
		store:     store,
		forward:   true,
		cursor:    cursor,
		rowCursor: newRowCursor(store, tx),
		filter:    query,
	}
	result.setPaging(query)
	result.Next()
	return result
}

func newFilteredCursor(tx *bbolt.Tx, store ListStore, cursor ast.SeekableSetCursor, filter ast.BoolNode) ast.SeekableSetCursor {
	result := &uniqueIndexScanner{
		scanner: scanner{
			targetOffset: 0,
			targetLimit:  math.MaxInt64,
		},
		store:     store,
		forward:   true,
		cursor:    cursor,
		rowCursor: newRowCursor(store, tx),
		filter:    filter,
	}

	if query, ok := filter.(ast.Query); ok {
		result.setPaging(query)
	}

	result.Next()
	return result
}

func (scanner *uniqueIndexScanner) Scan(tx *bbolt.Tx, query ast.Query) ([]string, int64, error) {
	entityBucket := scanner.store.GetEntitiesBucket(tx)
	if entityBucket == nil {
		return nil, 0, nil
	}
	return scanner.ScanCursor(tx, entityBucket.OpenCursor, query)
}

func (scanner *uniqueIndexScanner) ScanCursor(tx *bbolt.Tx, cursorProvider ast.SetCursorProvider, query ast.Query) ([]string, int64, error) {
	scanner.setPaging(query)
	scanner.rowCursor = newRowCursor(scanner.store, tx)
	scanner.filter = query
	scanner.cursor = cursorProvider(tx, scanner.forward)

	if scanner.cursor == nil {
		return nil, 0, nil
	}

	scanner.nextUnpaged()

	var result []string
	for scanner.IsValid() {
		id := scanner.Current()
		if scanner.offset < scanner.targetOffset {
			scanner.offset++
		} else {
			if scanner.collected < scanner.targetLimit {
				result = append(result, string(id))
				scanner.collected++
			}
		}
		scanner.count++
		scanner.nextUnpaged()
	}
	return result, scanner.count, nil
}

func (scanner *uniqueIndexScanner) IsValid() bool {
	return scanner.current != nil
}

func (scanner *uniqueIndexScanner) Current() []byte {
	return scanner.current
}

func (scanner *uniqueIndexScanner) Next() {
	cursor := scanner.cursor
	rowCursor := scanner.rowCursor
	for {
		if !cursor.IsValid() {
			scanner.current = nil
			return
		}

		if scanner.collected >= scanner.targetLimit {
			scanner.current = nil
			return
		}

		scanner.current = cursor.Current()
		cursor.Next()
		if scanner.store.IsChildStore() && !scanner.store.IsEntityPresent(rowCursor.Tx(), string(scanner.current)) && !scanner.store.IsExtended() {
			continue
		}
		rowCursor.NextRow(scanner.current)
		match := scanner.filter.EvalBool(rowCursor)
		if match {
			if scanner.offset < scanner.targetOffset {
				scanner.offset++
			} else {
				scanner.collected++
				return
			}
		}
	}
}

func (scanner *uniqueIndexScanner) nextUnpaged() {
	cursor := scanner.cursor
	rowCursor := scanner.rowCursor
	for {
		if !cursor.IsValid() {
			scanner.current = nil
			return
		}

		scanner.current = cursor.Current()
		cursor.Next()
		if scanner.store.IsChildStore() && !scanner.store.IsEntityPresent(rowCursor.Tx(), string(scanner.current)) && !scanner.store.IsExtended() {
			continue
		}
		rowCursor.NextRow(scanner.current)
		match := scanner.filter.EvalBool(rowCursor)
		if match {
			return
		}
	}
}

func (scanner *uniqueIndexScanner) Seek(val []byte) {
	cursor := scanner.cursor
	if seekableCursor, ok := cursor.(ast.SeekableSetCursor); ok {
		seekableCursor.Seek(val)
		scanner.Next()
	} else {
		for scanner.IsValid() && string(scanner.current) < string(val) {
			scanner.Next()
		}
	}
}

type sortingScanner struct {
	scanner
	store  ListStore
	offset int64
	count  int64
}

func (scanner *sortingScanner) Scan(tx *bbolt.Tx, query ast.Query) ([]string, int64, error) {
	entityBucket := scanner.store.GetEntitiesBucket(tx)
	if entityBucket == nil {
		return nil, 0, nil
	}
	return scanner.ScanCursor(tx, entityBucket.OpenCursor, query)
}

func (scanner *sortingScanner) ScanCursor(tx *bbolt.Tx, cursorProvider ast.SetCursorProvider, query ast.Query) ([]string, int64, error) {
	scanner.setPaging(query)
	comparator, err := scanner.store.NewRowComparator(query.GetSortFields())
	if err != nil {
		return nil, 0, err
	}

	rowCursor := newRowCursor(scanner.store, tx)
	rowContext := &RowContext{comparator: comparator, rowCursor1: rowCursor, rowCursor2: newRowCursor(scanner.store, tx)}

	cursor := cursorProvider(tx, true)

	if cursor == nil {
		return nil, 0, nil
	}

	// Longer term, if we're looking for better performance, we could make a version of llrb which takes a comparator
	// function instead of putting the comparison on the elements, so we don't need to store a context with each row
	results := &llrb.Tree{}
	isChildStore := scanner.store.IsChildStore()
	maxResults := scanner.targetOffset + scanner.targetLimit
	for cursor.IsValid() {
		current := cursor.Current()
		cursor.Next()
		if isChildStore && !scanner.store.IsEntityPresent(tx, string(current)) && !scanner.store.IsExtended() {
			continue
		}
		rowCursor.NextRow(current)
		if query.EvalBool(rowCursor) {
			results.Insert(&Row{id: current, context: rowContext})
			scanner.count++
			if scanner.count > maxResults {
				results.DeleteMax()
			}
		}
	}

	var result []string
	results.Do(func(row llrb.Comparable) bool {
		if scanner.offset < scanner.targetOffset {
			scanner.offset++
		} else {
			result = append(result, string(row.(*Row).id))
		}
		return false
	})

	return result, scanner.count, nil
}

type RowContext struct {
	comparator RowComparator
	rowCursor1 *rowCursorImpl
	rowCursor2 *rowCursorImpl
}

type Row struct {
	id      []byte
	context *RowContext
}

func (r *Row) Compare(other llrb.Comparable) int {
	r.context.rowCursor1.NextRow(r.id)
	r.context.rowCursor2.NextRow(other.(*Row).id)
	return r.context.comparator.Compare(r.context.rowCursor1, r.context.rowCursor2)
}
