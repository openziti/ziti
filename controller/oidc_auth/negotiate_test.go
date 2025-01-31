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
	"github.com/stretchr/testify/require"
	"testing"
)

func Test_parseContentTypeMediaType(t *testing.T) {
	t.Run("empty string returns empty string", func(t *testing.T) {
		req := require.New(t)

		result := parseContentTypeMediaType("")
		req.Equal("", result)
	})

	t.Run("a media type only string returns the media type", func(t *testing.T) {
		req := require.New(t)

		result := parseContentTypeMediaType("application/json")
		req.Equal("application/json", result)
	})

	t.Run("a media type with a charset string returns the media type", func(t *testing.T) {
		req := require.New(t)

		result := parseContentTypeMediaType("text/html; charset=UTF-8")
		req.Equal("text/html", result)
	})

	t.Run("a media type with extra space and a charset string returns the media type", func(t *testing.T) {
		req := require.New(t)

		result := parseContentTypeMediaType("            text/html;             charset=UTF-8           ")
		req.Equal("text/html", result)
	})

	t.Run("a media type with a boundary returns the media type", func(t *testing.T) {
		req := require.New(t)

		result := parseContentTypeMediaType("multipart/form-data; boundary=----WebKitFormBoundaryxyz")
		req.Equal("multipart/form-data", result)
	})
}

func Test_parseAcceptHeader(t *testing.T) {
	t.Run("an empty accept header returns nil", func(t *testing.T) {
		req := require.New(t)

		result := parseAcceptHeader("")
		req.Nil(result)

	})

	t.Run("an invalid media type returns the value provided", func(t *testing.T) {
		req := require.New(t)

		result := parseAcceptHeader("bobble")
		req.Len(result, 1)
		req.Equal("bobble", result[0].MediaType)
	})

	t.Run("*/* returns */*", func(t *testing.T) {
		req := require.New(t)

		result := parseAcceptHeader("*/*")
		req.Len(result, 1)
		req.Equal("*/*", result[0].MediaType)
	})

	t.Run("image/png, image/jpeg;q=0.8, image/*;q=0.5 returns correct values in correct order", func(t *testing.T) {
		req := require.New(t)

		result := parseAcceptHeader("image/png, image/*;q=0.5, image/jpeg;q=0.8")
		req.Len(result, 3)

		req.Equal("image/png", result[0].MediaType)
		req.Equal(1.0, result[0].QValue)

		req.Equal("image/jpeg", result[1].MediaType)
		req.Equal(0.8, result[1].QValue)

		req.Equal("image/*", result[2].MediaType)
		req.Equal(0.5, result[2].QValue)
	})

	t.Run("is not space sensitive", func(t *testing.T) {
		req := require.New(t)

		result := parseAcceptHeader("              image/png           ,image/*;q=0.5,image/jpeg;q=0.8           ")
		req.Len(result, 3)

		req.Equal("image/png", result[0].MediaType)
		req.Equal(1.0, result[0].QValue)

		req.Equal("image/jpeg", result[1].MediaType)
		req.Equal(0.8, result[1].QValue)

		req.Equal("image/*", result[2].MediaType)
		req.Equal(0.5, result[2].QValue)
	})
}

func Test_AcceptHeader_Match(t *testing.T) {
	t.Run("static main and sub type match", func(t *testing.T) {
		req := require.New(t)
		entry := &AcceptEntry{
			FullHeader: "application/json",
			MediaType:  "application/json",
			QValue:     1,
		}

		result := entry.Match("application/json")
		req.True(result)
	})

	t.Run("static main and sub types do not match if main is wrong", func(t *testing.T) {
		req := require.New(t)
		entry := &AcceptEntry{
			FullHeader: "application/json",
			MediaType:  "application/json",
			QValue:     1,
		}

		result := entry.Match("badValue/json")
		req.False(result)
	})

	t.Run("static main and sub types do not match if sub is wrong", func(t *testing.T) {
		req := require.New(t)
		entry := &AcceptEntry{
			FullHeader: "application/json",
			MediaType:  "application/json",
			QValue:     1,
		}
		result := entry.Match("application/badValue")
		req.False(result)
	})

	t.Run("static main and sub types do not match if main and sub are wrong", func(t *testing.T) {
		req := require.New(t)
		entry := &AcceptEntry{
			FullHeader: "application/json",
			MediaType:  "application/json",
			QValue:     1,
		}
		result := entry.Match("badValue/anotherBadValue")
		req.False(result)
	})

	t.Run("glob main type matches if static sub matches", func(t *testing.T) {
		req := require.New(t)
		entry := &AcceptEntry{
			FullHeader: "*/json",
			MediaType:  "*/json",
			QValue:     1,
		}

		result := entry.Match("whatever/json")
		req.True(result)
	})

	t.Run("glob main type does not match if static sub does not match", func(t *testing.T) {
		req := require.New(t)
		entry := &AcceptEntry{
			FullHeader: "*/json",
			MediaType:  "*/json",
			QValue:     1,
		}

		result := entry.Match("whatever/badValue")
		req.False(result)
	})

	t.Run("glob sub type matches if static main matches", func(t *testing.T) {
		req := require.New(t)
		entry := &AcceptEntry{
			FullHeader: "application/*",
			MediaType:  "application/*",
			QValue:     1,
		}

		result := entry.Match("application/whatever")
		req.True(result)
	})

	t.Run("glob sub type does not match if static main does not match", func(t *testing.T) {
		req := require.New(t)
		entry := &AcceptEntry{
			FullHeader: "application/*",
			MediaType:  "application/*",
			QValue:     1,
		}

		result := entry.Match("badValue/whatever")
		req.False(result)
	})

	t.Run("glob main and sub type matches any valid media type", func(t *testing.T) {
		req := require.New(t)
		entry := &AcceptEntry{
			FullHeader: "*/*",
			MediaType:  "*/*",
			QValue:     1,
		}

		result := entry.Match("whatever1/whatever2")
		req.True(result)
	})

	t.Run("glob main and sub type", func(t *testing.T) {
		entry := &AcceptEntry{
			FullHeader: "*/*",
			MediaType:  "*/*",
			QValue:     1,
		}

		t.Run("does not match no slash in media type", func(t *testing.T) {
			req := require.New(t)
			result := entry.Match("invalidValueNoSlash")
			req.False(result)
		})

		t.Run("does match too many slashes", func(t *testing.T) {
			req := require.New(t)

			//extra slahes in subtype are not split and instead make up the entire subtype (e.g. subType == "whatever2/whatever3")
			result := entry.Match("whatever1/whatever2/whatever3")
			req.True(result)
		})

		t.Run("does not match empty media type", func(t *testing.T) {
			req := require.New(t)
			result := entry.Match("")
			req.False(result)
		})

	})

	t.Run("an  invalid media type matches nothing", func(t *testing.T) {
		req := require.New(t)
		entry := &AcceptEntry{
			FullHeader: "iAmABadValue",
			MediaType:  "iAmABadValue",
			QValue:     1,
		}

		result := entry.Match("application/json")
		req.False(result)
	})
}
