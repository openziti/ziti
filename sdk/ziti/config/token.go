/*
	Copyright 2019 Netfoundry, Inc.

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

package config

import (
	"crypto/x509"
	"errors"
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/michaelquigley/pfxlog"
	"github.com/mitchellh/mapstructure"
	"net/url"
	"path"
	"reflect"
)

const (
	IdentityTypeGateway    = "gateway"
	IdentityTypeEndpoint   = "endpoint"
	IdentityTypeCaEndpoint = "ca-endpoint"

	AudienceGatewayEnroller  = "gateway-enroller"
	AudienceEndpointEnroller = "endpoint-enroller"
)

type Versions struct {
	Api           string `json:"api"`
	EnrollmentApi string `json:"enrollmentApi"`
}

type Identity struct {
	Type string `json:"type"`
	Id   string `json:"id"`
	Name string `json:"name"`
}

type EnrollmentClaims struct {
	EnrollmentMethod string            `json:"em"`
	SignatureCert    *x509.Certificate `json:"-"`
	jwt.StandardClaims
}

func (t *EnrollmentClaims) EnrolmentUrl() string {
	enrollmentUrl, err := url.Parse(t.Issuer)

	if err != nil {
		pfxlog.Logger().WithError(err).WithField("url", t.Issuer).Panic("could not parse issuer as URL")
	}

	enrollmentUrl.Path = path.Join(enrollmentUrl.Path, "enroll")
	query := enrollmentUrl.Query()
	query.Add("method", t.EnrollmentMethod)
	query.Add("token", t.Id)
	enrollmentUrl.RawQuery = query.Encode()
	
	return enrollmentUrl.String()
}

func (t *EnrollmentClaims) ToMapClaims() (jwt.MapClaims, error) {
	mapClaims := map[string]interface{}{}
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook:       nil,
		ErrorUnused:      false,
		ZeroFields:       false,
		WeaklyTypedInput: false,
		Metadata:         nil,
		Result:           &mapClaims, //map pointer required
		TagName:          "json",
	})

	if err != nil {
		return nil, fmt.Errorf("could not create decoder: %s", err)
	}

	if err = decoder.Decode(t); err != nil {
		return nil, fmt.Errorf("could not decode: %s", err)
	}

	if std, found := mapClaims["StandardClaims"]; found {
		delete(mapClaims, "StandardClaims")

		if stdMap, ok := std.(map[string]interface{}); ok {
			for k, v := range stdMap {
				if !isZeroValue(v) {
					mapClaims[k] = v
				}
			}
		} else {
			return nil, errors.New("could not converted standard claims section to map")
		}

	}

	return mapClaims, nil
}

func isZeroValue(x interface{}) bool {
	return x == reflect.Zero(reflect.TypeOf(x)).Interface()
}

func (t EnrollmentClaims) Valid() error {
	return t.StandardClaims.Valid()
}
