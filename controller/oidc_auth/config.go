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

package oidc_auth

import (
	"crypto"
	"crypto/sha256"
	"crypto/x509"
	"github.com/openziti/identity"
	"github.com/openziti/ziti/common"
	"time"
)

// Config represents the configuration necessary to operate an OIDC Provider
type Config struct {
	Issuers              []string
	TokenSecret          string
	Storage              Storage
	Certificate          *x509.Certificate
	PrivateKey           crypto.PrivateKey
	IdTokenDuration      time.Duration
	RefreshTokenDuration time.Duration
	AccessTokenDuration  time.Duration
	RedirectURIs         []string
	PostLogoutURIs       []string

	maxTokenDuration *time.Duration
	Identity         identity.Identity
}

// NewConfig will create a Config with default values
func NewConfig(issuers []string, cert *x509.Certificate, key crypto.PrivateKey) Config {
	return Config{
		Issuers:              issuers,
		Certificate:          cert,
		PrivateKey:           key,
		RefreshTokenDuration: common.DefaultRefreshTokenDuration,
		AccessTokenDuration:  common.DefaultAccessTokenDuration,
		IdTokenDuration:      common.DefaultIdTokenDuration,
	}
}

// MaxTokenDuration returns the maximum token lifetime currently configured
func (c *Config) MaxTokenDuration() time.Duration {
	if c.maxTokenDuration == nil {
		curMaxDur := c.RefreshTokenDuration

		for _, duration := range []time.Duration{c.AccessTokenDuration, c.IdTokenDuration} {
			if duration > curMaxDur {
				curMaxDur = duration
			}
		}

		c.maxTokenDuration = &curMaxDur
	}

	return *c.maxTokenDuration
}

// Secret returns a sha256 sum of the configured token secret
func (c *Config) Secret() [32]byte {
	return sha256.Sum256([]byte(c.TokenSecret))
}
