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

package common

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sirupsen/logrus"
)

// LoggingFields returns the fields that describe this service access token to a log or an
// audit trail: who it was minted for, what it grants access to, and when it is valid. The
// raw token is a credential and never appears among them. Pass the result to
// logrus.Entry.WithFields.
//
// A claim the token did not carry is left out rather than logged empty, so the result can be
// merged over a field set built from stored state without blanking it.
func (c *ServiceAccessClaims) LoggingFields() logrus.Fields {
	fields := logrus.Fields{}

	if c == nil {
		return fields
	}

	addLogString(fields, "sessionId", c.ID)
	addLogString(fields, "sessionType", c.Type)
	addLogString(fields, "identityId", c.IdentityId)
	addLogString(fields, "serviceId", c.Subject)
	addLogString(fields, "apiSessionId", c.ApiSessionId)
	addLogJwtTime(fields, "sessionIssuedAt", c.IssuedAt)
	addLogJwtTime(fields, "sessionNotBefore", c.NotBefore)
	addLogJwtTime(fields, "sessionExpiresAt", c.ExpiresAt)
	fields["sessionIsLegacy"] = c.IsLegacy

	return fields
}

// LoggingFields returns the fields that describe this access token to a log or an audit
// trail. The raw token is a credential and never appears among them. Pass the result to
// logrus.Entry.WithFields.
//
// A claim the token did not carry is left out rather than logged empty, so the result can be
// merged over a field set built from stored state without blanking it.
func (r *AccessClaims) LoggingFields() logrus.Fields {
	fields := logrus.Fields{}

	if r == nil {
		return fields
	}

	addLogString(fields, "apiSessionId", r.ApiSessionId)
	addLogString(fields, "apiSessionTokenId", r.JWTID)
	addLogString(fields, "apiSessionType", r.CustomClaims.Type)
	addLogString(fields, "apiSessionRemoteAddress", r.RemoteAddress)
	addLogString(fields, "identityId", r.Subject)
	addLogTime(fields, "apiSessionIssuedAt", r.IssuedAt.AsTime())
	addLogTime(fields, "apiSessionNotBefore", r.NotBefore.AsTime())
	addLogTime(fields, "apiSessionExpiresAt", r.Expiration.AsTime())

	return fields
}

// addLogString records a value, skipping it when it is empty.
func addLogString(fields logrus.Fields, name string, value string) {
	if value != "" {
		fields[name] = value
	}
}

// addLogJwtTime records a JWT time claim in RFC3339 form, skipping a claim the token did not
// carry.
func addLogJwtTime(fields logrus.Fields, name string, value *jwt.NumericDate) {
	if value != nil {
		addLogTime(fields, name, value.Time)
	}
}

// addLogTime records a timestamp in RFC3339 form, skipping the zero value, which means no
// time was carried rather than a time at the epoch.
func addLogTime(fields logrus.Fields, name string, value time.Time) {
	if !value.IsZero() {
		fields[name] = value.UTC().Format(time.RFC3339)
	}
}
