// Code generated by go-swagger; DO NOT EDIT.

//
// Copyright NetFoundry, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// __          __              _
// \ \        / /             (_)
//  \ \  /\  / /_ _ _ __ _ __  _ _ __   __ _
//   \ \/  \/ / _` | '__| '_ \| | '_ \ / _` |
//    \  /\  / (_| | |  | | | | | | | | (_| | : This file is generated, do not edit it.
//     \/  \/ \__,_|_|  |_| |_|_|_| |_|\__, |
//                                      __/ |
//                                     |___/

package rest_model

// This file was generated by the swagger tool.
// Editing this file might prove futile when you re-run the swagger generate command

import (
	"context"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/swag"
	"github.com/go-openapi/validate"
)

// RouterCreate router create
//
// swagger:model routerCreate
type RouterCreate struct {

	// cost
	// Required: true
	// Maximum: 65535
	// Minimum: 0
	Cost *int64 `json:"cost"`

	// fingerprint
	Fingerprint *string `json:"fingerprint,omitempty"`

	// id
	// Required: true
	ID *string `json:"id"`

	// name
	// Required: true
	Name *string `json:"name"`

	// tags
	Tags *Tags `json:"tags,omitempty"`
}

// Validate validates this router create
func (m *RouterCreate) Validate(formats strfmt.Registry) error {
	var res []error

	if err := m.validateCost(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateID(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateName(formats); err != nil {
		res = append(res, err)
	}

	if err := m.validateTags(formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *RouterCreate) validateCost(formats strfmt.Registry) error {

	if err := validate.Required("cost", "body", m.Cost); err != nil {
		return err
	}

	if err := validate.MinimumInt("cost", "body", *m.Cost, 0, false); err != nil {
		return err
	}

	if err := validate.MaximumInt("cost", "body", *m.Cost, 65535, false); err != nil {
		return err
	}

	return nil
}

func (m *RouterCreate) validateID(formats strfmt.Registry) error {

	if err := validate.Required("id", "body", m.ID); err != nil {
		return err
	}

	return nil
}

func (m *RouterCreate) validateName(formats strfmt.Registry) error {

	if err := validate.Required("name", "body", m.Name); err != nil {
		return err
	}

	return nil
}

func (m *RouterCreate) validateTags(formats strfmt.Registry) error {
	if swag.IsZero(m.Tags) { // not required
		return nil
	}

	if m.Tags != nil {
		if err := m.Tags.Validate(formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("tags")
			} else if ce, ok := err.(*errors.CompositeError); ok {
				return ce.ValidateName("tags")
			}
			return err
		}
	}

	return nil
}

// ContextValidate validate this router create based on the context it is used
func (m *RouterCreate) ContextValidate(ctx context.Context, formats strfmt.Registry) error {
	var res []error

	if err := m.contextValidateTags(ctx, formats); err != nil {
		res = append(res, err)
	}

	if len(res) > 0 {
		return errors.CompositeValidationError(res...)
	}
	return nil
}

func (m *RouterCreate) contextValidateTags(ctx context.Context, formats strfmt.Registry) error {

	if m.Tags != nil {
		if err := m.Tags.ContextValidate(ctx, formats); err != nil {
			if ve, ok := err.(*errors.Validation); ok {
				return ve.ValidateName("tags")
			} else if ce, ok := err.(*errors.CompositeError); ok {
				return ce.ValidateName("tags")
			}
			return err
		}
	}

	return nil
}

// MarshalBinary interface implementation
func (m *RouterCreate) MarshalBinary() ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return swag.WriteJSON(m)
}

// UnmarshalBinary interface implementation
func (m *RouterCreate) UnmarshalBinary(b []byte) error {
	var res RouterCreate
	if err := swag.ReadJSON(b, &res); err != nil {
		return err
	}
	*m = res
	return nil
}
