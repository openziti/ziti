package config

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func Test_validateHostPortString(t *testing.T) {
	t.Run("a hostname and port should pass", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("hello.com:123")

		req.NoError(err)
	})

	t.Run("an ipv4 localhost and port should pass", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("127.0.0.1:123")

		req.NoError(err)
	})

	t.Run("an ipv6 localhost and port should pass", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("[::1]:123")

		req.NoError(err)
	})

	t.Run("a hostname and a blank port should fail", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("hello.com:")

		req.Error(err)
	})

	t.Run("a blank hostname and a port should fail", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString(":123")

		req.Error(err)
	})

	t.Run("a blank hostname and a blank port should fail", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString(":")

		req.Error(err)
	})

	t.Run("too many colons with blank host and port should fail", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("::")

		req.Error(err)
	})

	t.Run("extra trailing colons with host and port should fail", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("myhost:123:")

		req.Error(err)
	})

	t.Run("extra middle colon with host and port should fail", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("myhost::123")

		req.Error(err)
	})

	t.Run("extra leading colon with host and port should fail", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString(":myhost::123")

		req.Error(err)
	})

	t.Run("extra leading colon with host and port should fail", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("")

		req.Error(err)
	})

	t.Run("host with scheme should fail", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("http://myhost:80")

		req.Error(err)
	})

	t.Run("host with scheme should fail", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("http://myhost:80")

		req.Error(err)
	})
	
	t.Run("host with non-integer port should fail", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("hello.com:nooooooooo")

		req.Error(err)
	})

	t.Run("host with negative port should fail", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("hello.com:-1")

		req.Error(err)
	})

	t.Run("host with 0 port should fail", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("hello.com:0")

		req.Error(err)
	})

	t.Run("host with 1 port should pass", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("hello.com:1")

		req.NoError(err)
	})

	t.Run("host with 65535 port should pass", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("hello.com:65535")

		req.NoError(err)
	})

	t.Run("host with 65535 port should pass", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("hello.com:65535")

		req.NoError(err)
	})

	t.Run("host and port with trailing space pass", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("   hello.com:65535   ")

		req.NoError(err)
	})

	t.Run("white space host with port with fails", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("   :65535   ")

		req.Error(err)
	})

	t.Run("white space post with host fails", func(t *testing.T) {
		req := require.New(t)

		err := validateHostPortString("myhost:           ")

		req.Error(err)
	})
}
