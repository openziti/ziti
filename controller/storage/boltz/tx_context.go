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
	"context"
	"go.etcd.io/bbolt"
)

type CommitAction interface {
	Exec()
}

type MutateContext interface {
	Tx() *bbolt.Tx
	setTx(tx *bbolt.Tx) MutateContext
	IsSystemContext() bool
	GetSystemContext() MutateContext
	Context() context.Context
	UpdateContext(func(ctx context.Context) context.Context) MutateContext
	GetChangeAuthor() string
}

func NewMutateContext(changeAuthor string, context context.Context) MutateContext {
	ctx := &mutateContext{
		ctx:          context,
		changeAuthor: changeAuthor,
	}
	return ctx
}

func NewTxMutateContext(context context.Context, tx *bbolt.Tx) MutateContext {
	ctx := &mutateContext{
		ctx: context,
	}
	ctx.setTx(tx)
	return ctx
}

type mutateContext struct {
	tx           *bbolt.Tx
	ctx          context.Context
	changeAuthor string
}

func (self *mutateContext) GetSystemContext() MutateContext {
	return NewSystemMutateContext(self)
}

func (self *mutateContext) IsSystemContext() bool {
	return false
}

func (self *mutateContext) Tx() *bbolt.Tx {
	return self.tx
}

func (self *mutateContext) setTx(tx *bbolt.Tx) MutateContext {
	self.tx = tx
	return self
}

func (self *mutateContext) Context() context.Context {
	return self.ctx
}

func (self *mutateContext) GetChangeAuthor() string {
	return self.changeAuthor
}

func (self *mutateContext) UpdateContext(f func(context.Context) context.Context) MutateContext {
	self.ctx = f(self.ctx)
	return self
}

func NewSystemMutateContext(ctx MutateContext) MutateContext {
	if ctx.IsSystemContext() {
		return ctx
	}
	return &systemMutateContext{
		wrapped: ctx,
	}
}

type systemMutateContext struct {
	wrapped MutateContext
}

func (self *systemMutateContext) GetSystemContext() MutateContext {
	return self
}

func (self *systemMutateContext) Tx() *bbolt.Tx {
	return self.wrapped.Tx()
}

func (self *systemMutateContext) setTx(tx *bbolt.Tx) MutateContext {
	return self.wrapped.setTx(tx)
}

func (self *systemMutateContext) IsSystemContext() bool {
	return true
}

func (self *systemMutateContext) Context() context.Context {
	return self.wrapped.Context()
}

func (self *systemMutateContext) UpdateContext(f func(context.Context) context.Context) MutateContext {
	return self.wrapped.UpdateContext(f)
}

func (self *systemMutateContext) GetChangeAuthor() string {
	return self.wrapped.GetChangeAuthor()
}
