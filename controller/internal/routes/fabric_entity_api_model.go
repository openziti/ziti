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

package routes

import (
	"path"

	"github.com/go-openapi/strfmt"
	"github.com/openziti/ziti/controller/models"
	"github.com/openziti/ziti/controller/rest_model"
)

func FabricEntityToRestModel(entity models.Entity, linkFactory FabricLinksFactory) rest_model.BaseEntity {
	id := entity.GetId()
	createdAt := strfmt.DateTime(entity.GetCreatedAt())
	updatedAt := strfmt.DateTime(entity.GetUpdatedAt())

	tags := rest_model.Tags{
		SubTags: entity.GetTags(),
	}
	ret := rest_model.BaseEntity{
		ID:        &id,
		CreatedAt: &createdAt,
		UpdatedAt: &updatedAt,
		Links:     linkFactory.Links(entity),
		Tags:      &tags,
	}

	if ret.Tags.SubTags == nil {
		ret.Tags.SubTags = map[string]interface{}{}
	}

	return ret
}

type FullFabricLinkFactory interface {
	FabricLinksFactory
	SelfFabricLinkFactory
}

type FabricLinksFactory interface {
	Links(entity LinkEntity) rest_model.Links
	EntityName() string
}

type SelfFabricLinkFactory interface {
	SelfLink(entity models.Entity) rest_model.Link
}

type CreateFabricLinkFactory interface {
	SelfLinkFromId(id string) rest_model.Link
}

func NewBasicFabricLinkFactory(entityName string) *BasicFabricLinkFactory {
	return &BasicFabricLinkFactory{entityName: entityName}
}

type BasicFabricLinkFactory struct {
	entityName string
}

func (factory *BasicFabricLinkFactory) SelfLinkFromId(id string) rest_model.Link {
	return NewFabricLink(factory.SelfUrlString(id))
}

func (factory *BasicFabricLinkFactory) SelfUrlString(id string) string {
	//path.Join will remove the ./ prefix in its "clean" operation
	return "./" + path.Join(factory.entityName, id)
}

func (factory *BasicFabricLinkFactory) SelfLink(entity LinkEntity) rest_model.Link {
	return NewFabricLink(factory.SelfUrlString(entity.GetId()))
}

func (factory *BasicFabricLinkFactory) Links(entity LinkEntity) rest_model.Links {
	return rest_model.Links{
		EntityNameSelf: factory.SelfLink(entity),
	}
}

func (factory *BasicFabricLinkFactory) NewNestedLink(entity LinkEntity, elem ...string) rest_model.Link {
	elem = append([]string{factory.SelfUrlString(entity.GetId())}, elem...)
	//path.Join will remove the ./ prefix in its "clean" operation
	return NewFabricLink("./" + path.Join(elem...))
}

func (factory *BasicFabricLinkFactory) EntityName() string {
	return factory.entityName
}

type LinkEntity interface {
	GetId() string
}

func ToFabricEntityRef(name string, entity LinkEntity, factory FabricLinksFactory) *rest_model.EntityRef {
	return &rest_model.EntityRef{
		Links:  factory.Links(entity),
		Entity: factory.EntityName(),
		ID:     entity.GetId(),
		Name:   name,
	}
}

func NewFabricLink(path string) rest_model.Link {
	href := strfmt.URI(path)
	return rest_model.Link{
		Href: &href,
	}
}
