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
	"fmt"
	"github.com/michaelquigley/pfxlog"
	"github.com/openziti/foundation/v2/stringz"
	"strings"

	"github.com/openziti/storage/ast"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

func (store *BaseStore) MakeSymbolPublic(symbol string) {
	if store.GetSymbol(symbol) != nil {
		store.publicSymbols = append(store.publicSymbols, symbol)
	} else {
		pfxlog.Logger().Errorf("%v can't mark unknown symbol %v public", store.GetEntityType(), symbol)
	}
}

func (store *BaseStore) GetPublicSymbols() []string {
	return store.publicSymbols
}

func (store *BaseStore) GetSymbolType(name string) (ast.NodeType, bool) {
	if symbol := store.GetSymbol(name); symbol != nil {
		return symbol.GetType(), true
	}
	return 0, false
}

func (store *BaseStore) GetSetSymbolTypes(name string) ast.SymbolTypes {
	if symbol := store.GetSymbol(name); symbol != nil {
		return symbol.GetLinkedType()
	}
	return nil
}

func (store *BaseStore) IsSet(name string) (bool, bool) {
	if symbol := store.GetSymbol(name); symbol != nil {
		return symbol.IsSet(), true
	}
	return false, false
}

// GetSymbol returns the symbol for the given name, or nil if the symbol doesn't exist
func (store *BaseStore) GetSymbol(name string) EntitySymbol {
	/*
		Types of symbols that we need to handle
		1. Local single values (employee.name)
		2. Local set values (sub-buckets of non-id keys) (myEntity.phoneNumbers)
		3. Composite single value symbols (employee.manager.name)
		4. Composite multi-value symbols (employee.directReports.phoneNumbers)
		5. Maps (employee.tags.location, employee.manager.tags.location, employee.directReports.tags.location)
	*/
	if result := store.symbols[name]; result != nil {
		// If it's a set symbol, make a runtime copy so we don't share cursor data. If we ever have a case where we
		// are evaluating the same symbol in multiple context, this will still break, but since currently any given
		// expression only involves a single symbol, this should not be a problem
		if setSymbol, ok := result.(EntitySetSymbol); ok {
			return setSymbol.GetRuntimeSymbol()
		}
		return result
	}

	// if it's a composite symbol, create a symbol on the fly to represent the name
	if index := strings.IndexRune(name, '.'); index > 0 {
		parts := strings.Split(name, ".")
		// If it's a map symbol, create that now and return it
		if mapSymbol := store.mapSymbols[parts[0]]; mapSymbol != nil {
			var prefix []string
			prefix = append(prefix, mapSymbol.prefix...)
			prefix = append(prefix, mapSymbol.key)
			if len(parts) > 2 {
				middle := parts[1 : len(parts)-1]
				prefix = append(prefix, middle...)
			}
			key := parts[len(parts)-1]
			return store.newEntitySymbol(name, mapSymbol.symbolType, key, nil, prefix...)
		}

		if result := store.GetSymbol(parts[0]); result != nil {
			linkedEntitySymbol, ok := result.(linkedEntitySymbol)
			if !ok || linkedEntitySymbol.getLinkedType() == nil {
				return nil // Can only have composite symbols if it's linked
			}
			subSymbolName := strings.Join(parts[1:], ".")
			rest := linkedEntitySymbol.getLinkedType().GetSymbol(subSymbolName)
			if rest == nil {
				return nil
			}
			return store.createCompositeEntitySymbol(name, linkedEntitySymbol, rest)
		}
	}

	return nil
}

func (store *BaseStore) createCompositeEntitySymbol(name string, first linkedEntitySymbol, rest EntitySymbol) EntitySymbol {
	ces, ok := rest.(compositeEntitySymbol)
	var chain []EntitySymbol
	if !ok {
		chain = []EntitySymbol{first, rest}
	} else {
		chain = []EntitySymbol{first}
		chain = append(chain, ces.getChain()...)
	}

	noneSet := true
	var last EntitySymbol
	var iterable []iterableEntitySymbol
	for _, symbol := range chain {
		if symbol.IsSet() {
			noneSet = false
		}
		if iterableSymbol, ok := symbol.(iterableEntitySymbol); ok {
			iterable = append(iterable, iterableSymbol)
		}
		last = symbol
	}
	// strip ids, since they are redundant
	if _, ok := last.(*entityIdSymbol); ok {
		chain = chain[:len(chain)-1]
		if len(chain) == 1 {
			return chain[0]
		}
		last = chain[len(chain)-1]
	}
	if noneSet {
		return &nonSetCompositeEntitySymbol{
			name:       name,
			symbolType: rest.GetType(),
			chain:      chain,
		}
	}
	if len(chain) == len(iterable) || last.IsSet() {
		return &compositeEntitySetSymbol{
			name:       name,
			symbolType: rest.GetType(),
			chain:      iterable,
			cursor:     nil,
			cursorLastF: func(tx *bbolt.Tx, key []byte) (fieldType FieldType, bytes []byte) {
				return GetTypeAndValue(key)
			},
		}
	}
	return &compositeEntitySetSymbol{
		name:        name,
		symbolType:  rest.GetType(),
		chain:       iterable,
		cursor:      nil,
		cursorLastF: last.Eval,
	}
}

func (store *BaseStore) addSymbol(name string, public bool, symbol EntitySymbol) EntitySymbol {
	store.symbols[name] = symbol
	if public {
		store.publicSymbols = append(store.publicSymbols, name)
	}
	return symbol
}

func (store *BaseStore) GrantSymbols(child ListStore) {
	for name, value := range store.symbols {
		public := stringz.Contains(store.publicSymbols, name)
		child.addSymbol(name, public, value)
	}
}

func (store *BaseStore) AddIdSymbol(name string, nodeType ast.NodeType) EntitySymbol {
	return store.addSymbol(name, true, &entityIdSymbol{
		store:      store,
		symbolType: nodeType,
		path:       []string{"id"},
	})
}

func (store *BaseStore) MapSymbol(name string, mapper SymbolMapper) {
	if symbol, found := store.symbols[name]; found {
		store.symbols[name] = &symbolMapWrapper{
			EntitySymbol: symbol,
			SymbolMapper: mapper,
		}
	}
}

func (store *BaseStore) AddSymbol(name string, nodeType ast.NodeType, prefix ...string) EntitySymbol {
	return store.AddSymbolWithKey(name, nodeType, name, prefix...)
}

func (store *BaseStore) AddFkSymbol(name string, linkedStore ListStore, prefix ...string) EntitySymbol {
	return store.AddFkSymbolWithKey(name, name, linkedStore, prefix...)
}
func (store *BaseStore) AddSymbolWithKey(name string, nodeType ast.NodeType, key string, prefix ...string) EntitySymbol {
	return store.addSymbol(name, true, store.newEntitySymbol(name, nodeType, key, nil, prefix...))
}

func (store *BaseStore) AddFkSymbolWithKey(name string, key string, linkedStore ListStore, prefix ...string) EntitySymbol {
	return store.addSymbol(name, true, store.newEntitySymbol(name, ast.NodeTypeString, key, linkedStore, prefix...))
}

func (store *BaseStore) AddMapSymbol(name string, nodeType ast.NodeType, key string, prefix ...string) {
	store.mapSymbols[name] = &entityMapSymbol{
		key:        key,
		symbolType: nodeType,
		prefix:     prefix,
	}
}

func (store *BaseStore) NewEntitySymbol(name string, nodeType ast.NodeType) EntitySymbol {
	return store.newEntitySymbol(name, nodeType, name, nil)
}

func (store *BaseStore) newEntitySymbol(name string, nodeType ast.NodeType, key string, linkedType ListStore, prefix ...string) *entitySymbol {
	var path []string
	var bucketF func(entityBucket *TypedBucket) *TypedBucket

	if len(prefix) == 0 {
		path = []string{key}
		bucketF = func(entityBucket *TypedBucket) *TypedBucket {
			return entityBucket
		}
	} else {
		path = append(path, prefix...)
		path = append(path, key)

		bucketF = func(entityBucket *TypedBucket) *TypedBucket {
			if entityBucket == nil {
				return nil
			}
			return entityBucket.GetPath(prefix...)
		}
	}
	return &entitySymbol{
		store:      store,
		name:       name,
		getBucketF: bucketF,
		symbolType: nodeType,
		prefix:     prefix,
		key:        key,
		path:       path,
		linkedType: linkedType,
	}
}

func (store *BaseStore) AddSetSymbol(name string, nodeType ast.NodeType) EntitySetSymbol {
	return store.addSetSymbol(name, nodeType, nil)
}

func (store *BaseStore) AddPublicSetSymbol(name string, nodeType ast.NodeType) EntitySetSymbol {
	result := store.addSetSymbol(name, nodeType, nil)
	store.publicSymbols = append(store.publicSymbols, name)
	return result
}

func (store *BaseStore) AddFkSetSymbol(name string, listStore ListStore) EntitySetSymbol {
	return store.addSetSymbol(name, ast.NodeTypeString, listStore)
}

func (store *BaseStore) addSetSymbol(name string, nodeType ast.NodeType, listStore ListStore) EntitySetSymbol {
	entitySymbol := store.newEntitySymbol(name, nodeType, name, listStore)
	result := &entitySetSymbolImpl{
		entitySymbol: entitySymbol,
	}
	store.symbols[name] = result
	return result
}

func (store *BaseStore) AddExtEntitySymbols() {
	store.AddIdSymbol(FieldId, ast.NodeTypeString)
	store.AddSymbol(FieldCreatedAt, ast.NodeTypeDatetime)
	store.AddSymbol(FieldUpdatedAt, ast.NodeTypeDatetime)
	store.AddMapSymbol(FieldTags, ast.NodeTypeAnyType, FieldTags)
	store.AddSymbol(FieldIsSystemEntity, ast.NodeTypeBool)
}

func (store *BaseStore) NewScanner(sort []ast.SortField) Scanner {
	if len(sort) > SortMax {
		sort = sort[:SortMax]
	}

	if len(sort) == 0 || sort[0].Symbol() == "id" {
		if len(sort) < 1 || sort[0].IsAscending() {
			return &uniqueIndexScanner{store: store, forward: true}
		}
		return &uniqueIndexScanner{store: store, forward: false}
	}
	return &sortingScanner{store: store}
}

type sortFieldImpl struct {
	name  string
	isAsc bool
}

func (sortField *sortFieldImpl) Symbol() string {
	return sortField.name
}

func (sortField *sortFieldImpl) IsAscending() bool {
	return sortField.isAsc
}

func (sortField *sortFieldImpl) String() string {
	if sortField.isAsc {
		return sortField.name
	}
	return fmt.Sprintf("%v DESC", sortField.name)
}

func (store *BaseStore) NewRowComparator(sort []ast.SortField) (RowComparator, error) {
	// always have id as last sort element. this way if other sorts come out equal, we still
	// can order on something, instead of having duplicates which causes rows to get discarded
	sort = append(sort, &sortFieldImpl{name: "id", isAsc: true})

	var symbolsComparators []symbolComparator
	for _, sortField := range sort {
		symbol, found := store.symbols[sortField.Symbol()]
		forward := sortField.IsAscending()
		if !found {
			return nil, errors.Errorf("no such sort field: %v", sortField.Symbol())
		}
		if symbol.IsSet() {
			return nil, errors.Errorf("invalid sort field: %v", sortField.Symbol())
		}

		var symbolComparator symbolComparator
		switch symbol.GetType() {
		case ast.NodeTypeBool:
			symbolComparator = &boolSymbolComparator{symbol: symbol, forward: forward}
		case ast.NodeTypeDatetime:
			symbolComparator = &datetimeSymbolComparator{symbol: symbol, forward: forward}
		case ast.NodeTypeFloat64:
			symbolComparator = &float64SymbolComparator{symbol: symbol, forward: forward}
		case ast.NodeTypeInt64:
			symbolComparator = &int64SymbolComparator{symbol: symbol, forward: forward}
		case ast.NodeTypeString:
			symbolComparator = &stringSymbolComparator{symbol: symbol, forward: forward}
		default:
			return nil, errors.Errorf("unsupported sort field type %v for field : %v", ast.NodeTypeName(symbol.GetType()), sortField.Symbol())
		}
		symbolsComparators = append(symbolsComparators, symbolComparator)
	}

	return &rowComparatorImpl{symbols: symbolsComparators}, nil
}

func (store *BaseStore) QueryIdsf(tx *bbolt.Tx, queryString string, args ...interface{}) ([]string, int64, error) {
	return store.QueryIds(tx, fmt.Sprintf(queryString, args...))
}

func (store *BaseStore) QueryIds(tx *bbolt.Tx, queryString string) ([]string, int64, error) {
	query, err := ast.Parse(store, queryString)
	if err != nil {
		return nil, 0, err
	}
	return store.QueryIdsC(tx, query)
}

func (store *BaseStore) QueryIdsC(tx *bbolt.Tx, query ast.Query) ([]string, int64, error) {
	scanner := store.NewScanner(query.GetSortFields())
	return scanner.Scan(tx, query)
}

func (store *BaseStore) IterateIds(tx *bbolt.Tx, filter ast.BoolNode) ast.SeekableSetCursor {
	entitiesBucket := store.GetEntitiesBucket(tx)
	if entitiesBucket == nil {
		return ast.EmptyCursor
	}
	cursor := newFilteredCursor(tx, store, entitiesBucket.OpenSeekableCursor(), filter)
	return cursor
}

func (store *BaseStore) IterateValidIds(tx *bbolt.Tx, filter ast.BoolNode) ast.SeekableSetCursor {
	result := store.IterateIds(tx, filter)
	if store.isExtended {
		validIdsCursor := &ValidIdsCursors{
			tx:      tx,
			store:   store,
			wrapped: result,
		}
		if validIdsCursor.IsValid() && !validIdsCursor.IsExtendedDataPresent() {
			validIdsCursor.Next()
		}
		result = validIdsCursor
	}
	return result
}

func (store *BaseStore) QueryWithCursorC(tx *bbolt.Tx, cursorProvider ast.SetCursorProvider, query ast.Query) ([]string, int64, error) {
	scanner := store.NewScanner(query.GetSortFields())
	return scanner.ScanCursor(tx, cursorProvider, query)
}

type ValidIdsCursors struct {
	tx      *bbolt.Tx
	store   *BaseStore
	wrapped ast.SeekableSetCursor
}

func (cursor *ValidIdsCursors) Seek(bytes []byte) {
	cursor.wrapped.Seek(bytes)
	for cursor.IsValid() && !cursor.IsExtendedDataPresent() {
		cursor.wrapped.Next()
	}
}

func (cursor *ValidIdsCursors) Next() {
	cursor.wrapped.Next()
	for cursor.IsValid() && !cursor.IsExtendedDataPresent() {
		cursor.wrapped.Next()
	}
}

func (cursor *ValidIdsCursors) IsExtendedDataPresent() bool {
	return nil != cursor.store.GetEntityBucket(cursor.tx, cursor.wrapped.Current())
}

func (cursor *ValidIdsCursors) IsValid() bool {
	return cursor.wrapped.IsValid()
}

func (cursor *ValidIdsCursors) Current() []byte {
	return cursor.wrapped.Current()
}
