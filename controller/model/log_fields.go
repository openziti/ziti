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

package model

import (
	"time"

	"github.com/sirupsen/logrus"
)

// LoggingFields returns the fields that identify this service session to a log or an audit
// trail: which identity holds it, which service it grants, and which api session it hangs
// off. The session token is a credential and never appears among them. Pass the result to
// logrus.Entry.WithFields.
//
// A field the session does not carry is left out rather than logged empty.
func (entity *Session) LoggingFields() logrus.Fields {
	fields := logrus.Fields{}

	if entity == nil {
		return fields
	}

	addLogString(fields, "sessionId", entity.Id)
	addLogString(fields, "sessionType", entity.Type)
	addLogString(fields, "identityId", entity.IdentityId)
	addLogString(fields, "serviceId", entity.ServiceId)
	addLogString(fields, "apiSessionId", entity.ApiSessionId)
	addLogTime(fields, "sessionCreatedAt", entity.CreatedAt)

	return fields
}

// LoggingFields returns the fields that identify this api session to a log or an audit
// trail. The api session token is a credential and never appears among them. Pass the
// result to logrus.Entry.WithFields.
//
// A field the api session does not carry is left out rather than logged empty.
func (entity *ApiSession) LoggingFields() logrus.Fields {
	fields := logrus.Fields{}

	if entity == nil {
		return fields
	}

	addLogString(fields, "apiSessionId", entity.Id)
	addLogString(fields, "identityId", entity.IdentityId)
	addLogString(fields, "apiSessionIpAddress", entity.IPAddress)
	addLogString(fields, "apiSessionAuthenticatorId", entity.AuthenticatorId)
	addLogTime(fields, "apiSessionExpiresAt", entity.ExpiresAt)
	addLogTime(fields, "apiSessionLastActivityAt", entity.LastActivityAt)

	return fields
}

// addLogString records a value, skipping it when it is empty.
func addLogString(fields logrus.Fields, name string, value string) {
	if value != "" {
		fields[name] = value
	}
}

// addLogTime records a timestamp in RFC3339 form, skipping the zero value, which means the
// entity carried no time rather than a time at the epoch.
func addLogTime(fields logrus.Fields, name string, value time.Time) {
	if !value.IsZero() {
		fields[name] = value.UTC().Format(time.RFC3339)
	}
}
