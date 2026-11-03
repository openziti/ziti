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
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPickValue(t *testing.T) {
	t.Run("returns the trimmed answer", func(t *testing.T) {
		answer, err := pickValue(strings.NewReader("  my-ca \r\n"), io.Discard, "CA name:", "ca", true)
		require.NoError(t, err)
		require.Equal(t, "my-ca", answer)
	})

	t.Run("empty answer takes the default", func(t *testing.T) {
		answer, err := pickValue(strings.NewReader("\n"), io.Discard, "CA name:", "ca", true)
		require.NoError(t, err)
		require.Equal(t, "ca", answer)
	})

	t.Run("required with no default prompts again", func(t *testing.T) {
		out := &strings.Builder{}
		answer, err := pickValue(strings.NewReader("\n\nfoo\n"), out, "CN:", "", true)
		require.NoError(t, err)
		require.Equal(t, "foo", answer)
		require.Equal(t, 2, strings.Count(out.String(), "a value is required"))
	})

	t.Run("optional accepts an empty answer", func(t *testing.T) {
		answer, err := pickValue(strings.NewReader("\n"), io.Discard, "CN:", "", false)
		require.NoError(t, err)
		require.Equal(t, "", answer)
	})

	t.Run("final line without newline is accepted", func(t *testing.T) {
		answer, err := pickValue(strings.NewReader("foo"), io.Discard, "CN:", "", true)
		require.NoError(t, err)
		require.Equal(t, "foo", answer)
	})

	t.Run("closed input is an error", func(t *testing.T) {
		_, err := pickValue(strings.NewReader("\n"), io.Discard, "CN:", "", true)
		require.ErrorIs(t, err, io.EOF)
	})

	t.Run("successive prompts each read one line", func(t *testing.T) {
		in := strings.NewReader("first\nsecond\n")
		first, err := pickValue(in, io.Discard, "a:", "", true)
		require.NoError(t, err)
		second, err := pickValue(in, io.Discard, "b:", "", true)
		require.NoError(t, err)
		require.Equal(t, "first", first)
		require.Equal(t, "second", second)
	})
}

func TestPickName(t *testing.T) {
	names := []string{"alpha", "beta", "gamma"}

	t.Run("no names returns empty without reading", func(t *testing.T) {
		name, err := pickName(strings.NewReader(""), io.Discard, nil, "pick:", "")
		require.NoError(t, err)
		require.Equal(t, "", name)
	})

	t.Run("single name returns it without reading", func(t *testing.T) {
		name, err := pickName(strings.NewReader(""), io.Discard, []string{"only"}, "pick:", "")
		require.NoError(t, err)
		require.Equal(t, "only", name)
	})

	t.Run("picks by number", func(t *testing.T) {
		name, err := pickName(strings.NewReader("2\n"), io.Discard, names, "pick:", "")
		require.NoError(t, err)
		require.Equal(t, "beta", name)
	})

	t.Run("picks by name", func(t *testing.T) {
		name, err := pickName(strings.NewReader("gamma\n"), io.Discard, names, "pick:", "")
		require.NoError(t, err)
		require.Equal(t, "gamma", name)
	})

	t.Run("a numeric name takes precedence over a number", func(t *testing.T) {
		name, err := pickName(strings.NewReader("2\n"), io.Discard, []string{"2", "other"}, "pick:", "")
		require.NoError(t, err)
		require.Equal(t, "2", name)
	})

	t.Run("empty answer takes the default", func(t *testing.T) {
		name, err := pickName(strings.NewReader("\n"), io.Discard, names, "pick:", "gamma")
		require.NoError(t, err)
		require.Equal(t, "gamma", name)
	})

	t.Run("empty answer without a default takes the first", func(t *testing.T) {
		name, err := pickName(strings.NewReader("\n"), io.Discard, names, "pick:", "")
		require.NoError(t, err)
		require.Equal(t, "alpha", name)
	})

	t.Run("invalid answers prompt again", func(t *testing.T) {
		out := &strings.Builder{}
		name, err := pickName(strings.NewReader("0\n4\ndelta\n3\n"), out, names, "pick:", "")
		require.NoError(t, err)
		require.Equal(t, "gamma", name)
		require.Equal(t, 3, strings.Count(out.String(), "enter a number from 1 to 3"))
	})

	t.Run("closed input is an error", func(t *testing.T) {
		_, err := pickName(strings.NewReader("delta\n"), io.Discard, names, "pick:", "")
		require.ErrorIs(t, err, io.EOF)
	})
}
