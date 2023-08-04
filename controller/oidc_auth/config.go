package oidc_auth

import (
	"crypto"
	"crypto/sha256"
	"crypto/x509"
	"time"
)

// Config represents the configuration necessary to operate an OIDC Provider
type Config struct {
	Issuer               string
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
}

// NewConfig will create a Config with default values
func NewConfig(issuer string, cert *x509.Certificate, key crypto.PrivateKey) Config {
	return Config{
		Issuer:               issuer,
		Certificate:          cert,
		PrivateKey:           key,
		RefreshTokenDuration: DefaultRefreshTokenDuration,
		AccessTokenDuration:  DefaultAccessTokenDuration,
		IdTokenDuration:      DefaultIdTokenDuration,
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
