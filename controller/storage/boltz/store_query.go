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
	"github.com/michaelquigley/pfxlog"
	"strings"

	"github.com/openziti/storage/ast"
	"github.com/pkg/errors"
	"go.etcd.io/bbolt"
)

func (store *BaseStore[E]) MakeSymbolPublic(symbol string) {
	if _, isMapSymbol := store.mapSymbols[symbol]; isMapSymbol || store.GetSymbol(symbol) != nil {
		store.publicSymbols[symbol] = struct{}{}
	} else {
		pfxlog.Logger().Errorf("%v can't mark unknown symbol %v public", store.GetEntityType(), symbol)
	}
}

func (store *BaseStore[E]) GetPublicSymbols() []string {
	var symbols []string
	for k := range store.publicSymbols {
		symbols = append(symbols, k)
	}
	return symbols
}

func (store *BaseStore[E]) IsPublicSymbol(symbol string) bool {
	if _, ok := store.publicSymbols[symbol]; ok {
		return true
	}
	if parts := strings.Split(symbol, "."); len(parts) > 1 {
		baseName := parts[0]
		if _, isMapSymbol := store.mapSymbols[baseName]; isMapSymbol {
			_, isPublic := store.publicSymbols[baseName]
			return isPublic
		}
	}
	return false
}

func (store *BaseStore[E]) GetSymbolType(name string) (ast.NodeType, bool) {
	if symbol := store.GetSymbol(name); symbol != nil {
		return symbol.GetType(), true
	}
	return 0, false
}

func (store *BaseStore[E]) GetSetSymbolTypes(name string) ast.SymbolTypes {
	if symbol := store.GetSymbol(name); symbol != nil {
		return symbol.GetLinkedType()
	}
	return nil
}

func (store *BaseStore[E]) IsSet(name string) (bool, bool) {
	if symbol := store.GetSymbol(name); symbol != nil {
		return symbol.IsSet(), true
	}
	return false, false
}

// GetSymbol returns the symbol for the given name, or nil if the symbol doesn't exist
func (store *BaseStore[E]) GetSymbol(name string) EntitySymbol {
	/*
		Types of symbols that we need to handle
		1. Local single values (employee.name)
		2. Local set values (sub-buckets of non-id keys) (myEntity.phoneNumbers)
		3. Composite single value symbols (employee.manager.name)
		4. Composite multi-value symbols (employee.directReports.phoneNumbers)
		5. Maps (employee.tags.location, employee.manager.tags.location, employee.directReports.tags.location)
	*/
	if result := store.symbols.Get(name); result != nil {
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
			return mapSymbol.createElementSymbol(name, parts)
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

func (store *BaseStore[E]) createCompositeEntitySymbol(name string, first linkedEntitySymbol, rest EntitySymbol) EntitySymbol {
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

func (store *BaseStore[E]) addSymbol(name string, public bool, symbol EntitySymbol) EntitySymbol {
	store.symbols.Put(name, symbol)
	if public {
		store.publicSymbols[name] = struct{}{}
	}
	return symbol
}

func (store *BaseStore[E]) inheritMapSymbol(symbol *entityMapSymbol) {
	store.mapSymbols[symbol.key] = symbol
}

func (store *BaseStore[E]) GrantSymbols(child ConfigurableStore) {
	for name, value := range store.symbols.AsMap() {
		child.addSymbol(name, store.IsPublicSymbol(name), value)
	}
	for name, value := range store.mapSymbols {
		child.inheritMapSymbol(value)
		if store.IsPublicSymbol(name) {
			child.MakeSymbolPublic(name)
		}
	}
}

func (store *BaseStore[E]) AddIdSymbol(name string, nodeType ast.NodeType) EntitySymbol {
	return store.addSymbol(name, true, &entityIdSymbol{
		store:      store,
		symbolType: nodeType,
		path:       []string{"id"},
	})
}

func (store *BaseStore[E]) MapSymbol(name string, mapper SymbolMapper) {
	if symbol := store.symbols.Get(name); symbol != nil {
		store.symbols.Put(name, &symbolMapWrapper{
			EntitySymbol: symbol,
			SymbolMapper: mapper,
		})
	}
}

func (store *BaseStore[E]) AddSymbol(name string, nodeType ast.NodeType, prefix ...string) EntitySymbol {
	return store.AddSymbolWithKey(name, nodeType, name, prefix...)
}

func (store *BaseStore[E]) AddFkSymbol(name string, linkedStore Store, prefix ...string) EntitySymbol {
	return store.AddFkSymbolWithKey(name, name, linkedStore, prefix...)
}
func (store *BaseStore[E]) AddSymbolWithKey(name string, nodeType ast.NodeType, key string, prefix ...string) EntitySymbol {
	return store.addSymbol(name, true, store.newEntitySymbol(name, nodeType, key, nil, prefix...))
}

func (store *BaseStore[E]) AddFkSymbolWithKey(name string, key string, linkedStore Store, prefix ...string) EntitySymbol {
	return store.addSymbol(name, true, store.newEntitySymbol(name, ast.NodeTypeString, key, linkedStore, prefix...))
}

func (store *BaseStore[E]) AddMapSymbol(name string, nodeType ast.NodeType, key string, prefix ...string) {
	store.mapSymbols[name] = &entityMapSymbol{
		store:      store,
		key:        key,
		symbolType: nodeType,
		prefix:     prefix,
	}
}

func (store *BaseStore[E]) AddEntitySymbol(symbol EntitySymbol) {
	store.symbols.Put(symbol.GetName(), symbol)
}

func (store *BaseStore[E]) NewEntitySymbol(name string, nodeType ast.NodeType) EntitySymbol {
	return store.newEntitySymbol(name, nodeType, name, nil)
}

func (store *BaseStore[E]) newEntitySymbol(name string, nodeType ast.NodeType, key string, linkedType Store, prefix ...string) *entitySymbol {
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

func (store *BaseStore[E]) AddSetSymbol(name string, nodeType ast.NodeType) EntitySetSymbol {
	return store.addSetSymbol(name, nodeType, nil)
}

func (store *BaseStore[E]) AddPublicSetSymbol(name string, nodeType ast.NodeType) EntitySetSymbol {
	result := store.addSetSymbol(name, nodeType, nil)
	store.publicSymbols[name] = struct{}{}
	return result
}

func (store *BaseStore[E]) AddFkSetSymbol(name string, listStore Store) EntitySetSymbol {
	return store.addSetSymbol(name, ast.NodeTypeString, listStore)
}

func (store *BaseStore[E]) addSetSymbol(name string, nodeType ast.NodeType, listStore Store) EntitySetSymbol {
	entitySymbol := store.newEntitySymbol(name, nodeType, name, listStore)
	result := &entitySetSymbolImpl{
		entitySymbol: entitySymbol,
	}
	store.symbols.Put(name, result)
	return result
}

func (store *BaseStore[E]) AddExtEntitySymbols() {
	store.AddIdSymbol(FieldId, ast.NodeTypeString)
	store.AddSymbol(FieldCreatedAt, ast.NodeTypeDatetime)
	store.AddSymbol(FieldUpdatedAt, ast.NodeTypeDatetime)
	store.AddMapSymbol(FieldTags, ast.NodeTypeAnyType, FieldTags)
	store.MakeSymbolPublic(FieldTags)
	store.AddSymbol(FieldIsSystemEntity, ast.NodeTypeBool)
}

func (store *BaseStore[E]) NewScanner(sort []ast.SortField) Scanner {
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

func (store *BaseStore[E]) newRowComparator(sort []ast.SortField) (RowComparator, error) {
	// always have id as last sort element. this way if other sorts come out equal, we still
	// can order on something, instead of having duplicates which causes rows to get discarded
	sort = append(sort, ast.NewSortFieldNode("id", true))

	var symbolsComparators []symbolComparator
	for _, sortField := range sort {
		symbol := store.symbols.Get(sortField.Symbol())
		forward := sortField.IsAscending()
		if symbol == nil {
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

func (store *BaseStore[E]) QueryIds(tx *bbolt.Tx, queryString string) ([]string, int64, error) {
	query, err := ast.Parse(store, queryString)
	if err != nil {
		return nil, 0, err
	}
	return store.QueryIdsC(tx, query)
}

func (store *BaseStore[E]) QueryIdsC(tx *bbolt.Tx, query ast.Query) ([]string, int64, error) {
	scanner := store.NewScanner(query.GetSortFields())
	return scanner.Scan(tx, query)
}

func (store *BaseStore[E]) IterateIds(tx *bbolt.Tx, filter ast.BoolNode) ast.SeekableSetCursor {
	entitiesBucket := store.GetEntitiesBucket(tx)
	if entitiesBucket == nil {
		return ast.EmptyCursor
	}
	cursor := newFilteredCursor(tx, store, entitiesBucket.OpenSeekableCursor(), filter)
	return cursor
}

func (store *BaseStore[E]) IterateValidIds(tx *bbolt.Tx, filter ast.BoolNode) ast.SeekableSetCursor {
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

func (store *BaseStore[E]) QueryWithCursorC(tx *bbolt.Tx, cursorProvider ast.SetCursorProvider, query ast.Query) ([]string, int64, error) {
	scanner := store.NewScanner(query.GetSortFields())
	return scanner.ScanCursor(tx, cursorProvider, query)
}

type ValidIdsCursors struct {
	tx      *bbolt.Tx
	store   Store
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
