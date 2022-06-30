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

package util

import (
	"fmt"
)

const (
	appName     = "ziti"
	sessionType = "edge-controller-session"
)

// Session stores configuration options for the CLI
type Session struct {
	Host  string
	Token string
	Cert  string
}

func (session *Session) GetBaseUrl() string {
	return session.Host
}

func (session *Session) GetCert() string {
	return session.Cert
}

func (session *Session) GetToken() string {
	return session.Token
}

// Persist writes out the Ziti CLI session file
func (session *Session) Persist() error {
	return WriteZitiAppFile(appName, sessionType, session)
}

// Load reads in the Ziti CLI session file
func (session *Session) Load() error {
	err := ReadZitiAppFile(appName, sessionType, session)
	if err != nil {
		return fmt.Errorf("unable to load Ziti CLI configuration. Exiting. Error: %v", err)
	}
	if session.Host == "" {
		return fmt.Errorf("host not specififed in cli config file. Exiting")
	}
	return nil
}

func (session *Session) String() string {
	return fmt.Sprintf("session Host: %v, Token: %s, Cert: %s", session.Host, session.Token, session.Cert)
}
