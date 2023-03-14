/*
	Copyright NetFoundry Inc.

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

package change

import (
	"context"
	"github.com/openziti/fabric/pb/cmd_pb"
	"github.com/openziti/storage/boltz"
)

type ContextKeyType string

const (
	ContextKey ContextKeyType = "changeContext"

	AuthorIdKey   = "authorId"
	AuthorNameKey = "authorName"
	TraceIdKey    = "traceId"
	Source        = "source"
)

func New() *Context {
	return &Context{
		Attributes: map[string]string{},
	}
}

type Context struct {
	Attributes map[string]string
	RaftIndex  uint64
}

func (self *Context) SetChangeAuthorId(val string) *Context {
	self.Attributes[AuthorIdKey] = val
	return self
}

func (self *Context) SetChangeAuthorName(val string) *Context {
	self.Attributes[AuthorNameKey] = val
	return self
}

func (self *Context) SetTraceId(val string) *Context {
	self.Attributes[TraceIdKey] = val
	return self
}

func (self *Context) SetSource(val string) *Context {
	self.Attributes[Source] = val
	return self
}

func (self *Context) ToProtoBuf() *cmd_pb.ChangeContext {
	if self == nil {
		return nil
	}
	return &cmd_pb.ChangeContext{
		Attributes: self.Attributes,
		RaftIndex:  self.RaftIndex,
	}
}

func (self *Context) GetContext() context.Context {
	return self.AddToContext(context.Background())
}

func (self *Context) NewMutateContext() boltz.MutateContext {
	return boltz.NewMutateContext(self.AddToContext(context.Background()))
}

func (self *Context) AddToContext(ctx context.Context) context.Context {
	if self == nil {
		return ctx
	}
	return context.WithValue(ctx, ContextKey, self)
}

func FromContext(ctx context.Context) *Context {
	val := ctx.Value(ContextKey)
	if val == nil {
		return nil
	}
	if changeContext, ok := val.(*Context); ok {
		return changeContext
	}
	return nil
}

func FromProtoBuf(ctx *cmd_pb.ChangeContext) *Context {
	if ctx == nil {
		return New()
	}
	result := &Context{
		Attributes: ctx.Attributes,
		RaftIndex:  ctx.RaftIndex,
	}
	if result.Attributes == nil {
		result.Attributes = map[string]string{}
	}
	return result
}
