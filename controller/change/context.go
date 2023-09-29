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
	"github.com/openziti/fabric/common/pb/cmd_pb"
	"github.com/openziti/storage/boltz"
)

type ContextKeyType string

const (
	ContextKey ContextKeyType = "changeContext"

	AuthorIdKey   = "authorId"
	AuthorNameKey = "authorName"
	AuthorTypeKey = "authorType"
	TraceIdKey    = "traceId"
	SourceType    = "src.type"
	SourceAuth    = "src.auth"
	SourceMethod  = "src.method"
	SourceLocal   = "src.local"
	SourceRemote  = "src.remote"
)

type AuthorType string

const (
	AuthorTypeCert         = "cert"
	AuthorTypeIdentity     = "identity"
	AuthorTypeRouter       = "router"
	AuthorTypeController   = "controller"
	AuthorTypeUnattributed = "unattributed"
)

const (
	SourceTypeControlChannel = "ctrl.channel"
	SourceTypeRest           = "rest"
	SourceTypeXt             = "xt"
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

type Author struct {
	Type string `json:"type"`
	Id   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type Source struct {
	Type       string `json:"type"`
	Auth       string `json:"auth,omitempty"`
	LocalAddr  string `json:"localAddr,omitempty"`
	RemoteAddr string `json:"remoteAddr,omitempty"`
	Method     string `json:"method,omitempty"`
}

func (self *Context) SetChangeAuthorType(val AuthorType) *Context {
	self.Attributes[AuthorTypeKey] = string(val)
	return self
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

func (self *Context) SetSourceType(val string) *Context {
	self.Attributes[SourceType] = val
	return self
}

func (self *Context) SetSourceAuth(val string) *Context {
	self.Attributes[SourceAuth] = val
	return self
}

func (self *Context) SetSourceMethod(val string) *Context {
	self.Attributes[SourceMethod] = val
	return self
}

func (self *Context) SetSourceLocal(val string) *Context {
	self.Attributes[SourceLocal] = val
	return self
}

func (self *Context) SetSourceRemote(val string) *Context {
	self.Attributes[SourceRemote] = val
	return self
}

func (self *Context) GetAuthor() *Author {
	if self == nil {
		return nil
	}
	return &Author{
		Type: self.Attributes[AuthorTypeKey],
		Id:   self.Attributes[AuthorIdKey],
		Name: self.Attributes[AuthorNameKey],
	}
}

func (self *Context) GetSource() *Source {
	if self == nil {
		return nil
	}
	return &Source{
		Type:       self.Attributes[SourceType],
		Auth:       self.Attributes[SourceAuth],
		LocalAddr:  self.Attributes[SourceLocal],
		RemoteAddr: self.Attributes[SourceRemote],
		Method:     self.Attributes[SourceMethod],
	}
}

func (self *Context) PopulateMetadata(meta map[string]any) {
	meta["author"] = self.GetAuthor()
	meta["source"] = self.GetSource()
	if traceId, found := self.Attributes[TraceIdKey]; found {
		meta["trace_id"] = traceId
	}
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
