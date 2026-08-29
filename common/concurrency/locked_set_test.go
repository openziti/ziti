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

package concurrency

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLockedSet_AddRemoveContains(t *testing.T) {
	req := require.New(t)
	set := NewLockedSet[string]()

	req.False(set.Contains("a"))
	req.Empty(set.Values())

	set.Add("a")
	set.Add("a")
	req.True(set.Contains("a"))
	req.Equal([]string{"a"}, set.Values(), "adding a value already present must not duplicate it")

	set.Remove("a")
	set.Remove("a")
	req.False(set.Contains("a"), "removing a value not present must be a no-op")
	req.Empty(set.Values())
}

func TestLockedSet_RemoveIf(t *testing.T) {
	req := require.New(t)
	set := NewLockedSet[string]()
	set.Add("a")

	set.RemoveIf("a", func() bool { return false })
	req.True(set.Contains("a"), "a refused removal must leave the value")

	set.RemoveIf("a", func() bool { return true })
	req.False(set.Contains("a"))

	// The predicate decides only whether to remove; it is not consulted for a value that is absent, since
	// there is nothing to remove either way.
	consulted := false
	set.RemoveIf("missing", func() bool {
		consulted = true
		return true
	})
	req.False(consulted, "the predicate must not run for a value that is not in the set")
}

func TestLockedSet_ValuesIsASnapshot(t *testing.T) {
	req := require.New(t)
	set := NewLockedSet[string]()
	set.Add("a")

	values := set.Values()
	set.Add("b")
	set.Remove("a")

	req.Equal([]string{"a"}, values, "a returned slice must not track later changes")
	req.Equal([]string{"b"}, set.Values())
}

func TestLockedSet_ConcurrentUse(t *testing.T) {
	set := NewLockedSet[int]()

	const writers = 8
	const perWriter = 500

	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(base int) {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				value := base*perWriter + i
				set.Add(value)
				set.Contains(value)
				set.RemoveIf(value, func() bool { return value%2 == 0 })
				set.Values()
			}
		}(w)
	}
	wg.Wait()

	for w := 0; w < writers; w++ {
		for i := 0; i < perWriter; i++ {
			value := w*perWriter + i
			require.Equal(t, value%2 != 0, set.Contains(value), fmt.Sprintf("value %d", value))
		}
	}
}
