package oidc_auth

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func Test_GetHandledHostNames(t *testing.T) {

	t.Run("localhost:443 returns all local addresses and supports implicit 443 port", func(t *testing.T) {
		result := getHandledHostnames("localhost:443")

		req := require.New(t)

		req.Len(result, 6)
		req.Contains(result, "localhost:443")
		req.Contains(result, "localhost")
		req.Contains(result, "127.0.0.1:443")
		req.Contains(result, "127.0.0.1")
		req.Contains(result, "[::1]")
		req.Contains(result, "[::1]:443")
	})
	t.Run("localhost:1234 returns all local addresses", func(t *testing.T) {
		result := getHandledHostnames("localhost:1234")

		req := require.New(t)

		req.Len(result, 3)
		req.Contains(result, "localhost:1234")
		req.Contains(result, "127.0.0.1:1234")
		req.Contains(result, "[::1]:1234")
	})

	t.Run("127.0.0.1:443 returns all local addresses and supports implicit 443 port", func(t *testing.T) {
		result := getHandledHostnames("127.0.0.1:443")

		req := require.New(t)

		req.Len(result, 6)
		req.Contains(result, "localhost:443")
		req.Contains(result, "localhost")
		req.Contains(result, "127.0.0.1:443")
		req.Contains(result, "127.0.0.1")
		req.Contains(result, "[::1]")
		req.Contains(result, "[::1]:443")
	})
	t.Run("127.0.0.1:1234 returns all local addresses", func(t *testing.T) {
		result := getHandledHostnames("127.0.0.1:1234")

		req := require.New(t)

		req.Len(result, 3)
		req.Contains(result, "localhost:1234")
		req.Contains(result, "127.0.0.1:1234")
		req.Contains(result, "[::1]:1234")
	})

	t.Run("[::1]:443 returns all local addresses and supports implicit 443 port", func(t *testing.T) {
		result := getHandledHostnames("[::1]:443")

		req := require.New(t)

		req.Len(result, 6)
		req.Contains(result, "localhost:443")
		req.Contains(result, "localhost")
		req.Contains(result, "127.0.0.1:443")
		req.Contains(result, "127.0.0.1")
		req.Contains(result, "[::1]")
		req.Contains(result, "[::1]:443")
	})
	t.Run("[::1]:1234 returns all local addresses", func(t *testing.T) {
		result := getHandledHostnames("[::1]:1234")

		req := require.New(t)

		req.Len(result, 3)
		req.Contains(result, "localhost:1234")
		req.Contains(result, "127.0.0.1:1234")
		req.Contains(result, "[::1]:1234")
	})
}
