package network

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNetwork_parseServiceAndIdentity(t *testing.T) {
	req := require.New(t)
	identity, serviceId := parseIdentityAndService("hello")
	req.Equal("", identity)
	req.Equal("hello", serviceId)

	identity, serviceId = parseIdentityAndService("@hello")
	req.Equal("", identity)
	req.Equal("hello", serviceId)

	identity, serviceId = parseIdentityAndService("a@hello")
	req.Equal("a", identity)
	req.Equal("hello", serviceId)

	identity, serviceId = parseIdentityAndService("bar@hello")
	req.Equal("bar", identity)
	req.Equal("hello", serviceId)

	identity, serviceId = parseIdentityAndService("@@hello")
	req.Equal("", identity)
	req.Equal("@hello", serviceId)

	identity, serviceId = parseIdentityAndService("a@@hello")
	req.Equal("a", identity)
	req.Equal("@hello", serviceId)

	identity, serviceId = parseIdentityAndService("a@foo@hello")
	req.Equal("a", identity)
	req.Equal("foo@hello", serviceId)
}
