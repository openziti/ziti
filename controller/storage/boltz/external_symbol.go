package boltz

import (
	"github.com/openziti/storage/ast"
	"go.etcd.io/bbolt"
)

type ExternalSymbol struct {
	store    Store
	name     string
	nodeType ast.NodeType
	impl     func(tx *bbolt.Tx, rowId []byte) (FieldType, []byte)
}

func (self *ExternalSymbol) GetStore() Store {
	return self.store
}

func (self *ExternalSymbol) GetLinkedType() Store {
	return nil
}

func (self *ExternalSymbol) GetPath() []string {
	return nil
}

func (self *ExternalSymbol) GetType() ast.NodeType {
	return self.nodeType
}

func (self *ExternalSymbol) GetName() string {
	return self.name
}

func (self *ExternalSymbol) IsSet() bool {
	return false
}

func (self *ExternalSymbol) Eval(tx *bbolt.Tx, rowId []byte) (FieldType, []byte) {
	return self.impl(tx, rowId)
}

func NewBoolFuncSymbol(store Store, name string, f func(id string) bool) EntitySymbol {
	return &ExternalSymbol{
		store:    store,
		name:     name,
		nodeType: ast.NodeTypeBool,
		impl: func(tx *bbolt.Tx, rowId []byte) (FieldType, []byte) {
			result := f(string(rowId))
			buf := make([]byte, 1)
			if result {
				buf[0] = 1
			}
			return TypeBool, buf
		},
	}
}

func NewStringFuncSymbol(store Store, name string, f func(id string) *string) EntitySymbol {
	return &ExternalSymbol{
		store:    store,
		name:     name,
		nodeType: ast.NodeTypeString,
		impl: func(tx *bbolt.Tx, rowId []byte) (FieldType, []byte) {
			result := f(string(rowId))
			if result == nil {
				return TypeString, nil
			}
			return TypeString, []byte(*result)
		},
	}
}
