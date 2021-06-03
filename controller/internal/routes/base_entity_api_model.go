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

package routes

import (
	"github.com/go-openapi/strfmt"
	"github.com/openziti/edge/rest_model"
	"github.com/openziti/fabric/controller/models"
	"path"
)

func BaseEntityToRestModel(entity models.Entity, linkFactory LinksFactory) rest_model.BaseEntity {
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
		Tags:     &tags,
	}

	if ret.Tags.SubTags == nil {
		ret.Tags.SubTags = map[string]interface{}{}
	}

	return ret
}

type FullLinkFactory interface {
	LinksFactory
	SelfLinkFactory
}

type LinksFactory interface {
	Links(entity models.Entity) rest_model.Links
	EntityName() string
}

type SelfLinkFactory interface {
	SelfLink(entity models.Entity) rest_model.Link
}

type CreateLinkFactory interface {
	SelfLinkFromId(id string) rest_model.Link
}

func NewBasicLinkFactory(entityName string) *BasicLinkFactory {
	return &BasicLinkFactory{entityName: entityName}
}

type BasicLinkFactory struct {
	entityName string
}

func (factory *BasicLinkFactory) SelfLinkFromId(id string) rest_model.Link {
	return NewLink(factory.SelfUrlString(id))
}

func (factory *BasicLinkFactory) SelfUrlString(id string) string {
	//path.Join will remove the ./ prefix in its "clean" operation
	return "./" + path.Join(factory.entityName, id)
}

func (factory *BasicLinkFactory) SelfLink(entity models.Entity) rest_model.Link {
	return NewLink(factory.SelfUrlString(entity.GetId()))
}

func (factory *BasicLinkFactory) Links(entity models.Entity) rest_model.Links {
	return rest_model.Links{
		EntityNameSelf: factory.SelfLink(entity),
	}
}

func (factory BasicLinkFactory) NewNestedLink(entity models.Entity, elem ...string) rest_model.Link {
	elem = append([]string{factory.SelfUrlString(entity.GetId())}, elem...)
	//path.Join will remove the ./ prefix in its "clean" operation
	return NewLink("./" + path.Join(elem...))
}

func (factory *BasicLinkFactory) EntityName() string {
	return factory.entityName
}

func ToEntityRef(name string, entity models.Entity, factory LinksFactory) *rest_model.EntityRef {
	return &rest_model.EntityRef{
		Links:  factory.Links(entity),
		Entity: factory.EntityName(),
		ID:     entity.GetId(),
		Name:   name,
	}
}

func NewLink(path string) rest_model.Link {
	href := strfmt.URI(path)
	return rest_model.Link{
		Href: &href,
	}
}
