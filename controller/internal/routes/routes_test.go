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

package routes

import (
	"testing"

	"github.com/openziti/edge-api/rest_model"
	"github.com/stretchr/testify/require"
)

func TestTagsOrDefault(t *testing.T) {
	tests := []struct {
		name     string
		input    *rest_model.Tags
		expected map[string]interface{}
	}{
		{
			name:     "nil wrapper yields empty map",
			input:    nil,
			expected: map[string]interface{}{},
		},
		{
			name:     "nil sub-tags yields empty map",
			input:    &rest_model.Tags{},
			expected: map[string]interface{}{},
		},
		{
			name:     "empty sub-tags yields empty map",
			input:    &rest_model.Tags{SubTags: rest_model.SubTags{}},
			expected: map[string]interface{}{},
		},
		{
			name:     "populated sub-tags are passed through",
			input:    &rest_model.Tags{SubTags: rest_model.SubTags{"managed": "true", "count": 3, "enabled": false}},
			expected: map[string]interface{}{"managed": "true", "count": 3, "enabled": false},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := TagsOrDefault(test.input)
			require.NotNil(t, result)
			require.Equal(t, test.expected, result)
		})
	}
}
