package network

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNetwork_parseServiceAndIdentity(t *testing.T) {
	req := require.New(t)
	identity, serviceId := parseServiceAndIdentity("hello")
	req.Equal("", identity)
	req.Equal("hello", serviceId)

	identity, serviceId = parseServiceAndIdentity("@hello")
	req.Equal("", identity)
	req.Equal("hello", serviceId)

	identity, serviceId = parseServiceAndIdentity("a@hello")
	req.Equal("a", identity)
	req.Equal("hello", serviceId)

	identity, serviceId = parseServiceAndIdentity("bar@hello")
	req.Equal("bar", identity)
	req.Equal("hello", serviceId)

	identity, serviceId = parseServiceAndIdentity("@@hello")
	req.Equal("", identity)
	req.Equal("@hello", serviceId)

	identity, serviceId = parseServiceAndIdentity("a@@hello")
	req.Equal("a", identity)
	req.Equal("@hello", serviceId)

	identity, serviceId = parseServiceAndIdentity("a@foo@hello")
	req.Equal("a", identity)
	req.Equal("foo@hello", serviceId)
}
