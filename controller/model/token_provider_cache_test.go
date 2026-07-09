package model

import (
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func Test_resolveStringSliceClaimProperty(t *testing.T) {
	t.Run("returns empty when the selector is unset", func(t *testing.T) {
		req := require.New(t)

		vals, err := resolveStringSliceClaimProperty(jwt.MapClaims{"roles": "admin"}, "")

		req.NoError(err)
		req.Empty(vals)
	})

	t.Run("returns empty when the claim is absent at the selected path", func(t *testing.T) {
		req := require.New(t)

		vals, err := resolveStringSliceClaimProperty(jwt.MapClaims{"name": "bob"}, "/roles")

		req.NoError(err)
		req.Empty(vals)
	})

	t.Run("returns empty when a nested claim is absent at the selected path", func(t *testing.T) {
		req := require.New(t)

		claims := jwt.MapClaims{"resource_access": map[string]any{"other": "x"}}

		vals, err := resolveStringSliceClaimProperty(claims, "/resource_access/ziti/roles")

		req.NoError(err)
		req.Empty(vals)
	})

	t.Run("returns empty when the claim is present but an empty string", func(t *testing.T) {
		req := require.New(t)

		vals, err := resolveStringSliceClaimProperty(jwt.MapClaims{"roles": ""}, "/roles")

		req.NoError(err)
		req.Empty(vals)
	})

	t.Run("returns a single value when the claim is a string", func(t *testing.T) {
		req := require.New(t)

		vals, err := resolveStringSliceClaimProperty(jwt.MapClaims{"roles": "admin"}, "/roles")

		req.NoError(err)
		req.Equal([]string{"admin"}, vals)
	})

	t.Run("returns all values when the claim is a string array", func(t *testing.T) {
		req := require.New(t)

		claims := jwt.MapClaims{"roles": []any{"admin", "support"}}

		vals, err := resolveStringSliceClaimProperty(claims, "/roles")

		req.NoError(err)
		req.Equal([]string{"admin", "support"}, vals)
	})

	t.Run("resolves a nested claim that is present", func(t *testing.T) {
		req := require.New(t)

		claims := jwt.MapClaims{"resource_access": map[string]any{"ziti": map[string]any{"roles": []any{"admin"}}}}

		vals, err := resolveStringSliceClaimProperty(claims, "/resource_access/ziti/roles")

		req.NoError(err)
		req.Equal([]string{"admin"}, vals)
	})

	t.Run("errors when the claim is present but not a string or array of strings", func(t *testing.T) {
		req := require.New(t)

		claims := jwt.MapClaims{"roles": map[string]any{"unexpected": "object"}}

		_, err := resolveStringSliceClaimProperty(claims, "/roles")

		req.Error(err)
	})
}
