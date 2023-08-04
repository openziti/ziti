package health

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestRound(t *testing.T) {
	interval := time.Millisecond * 100

	for i := 0; i < 100; i++ {
		now := time.Now()
		val := roundToClosest(now, interval)
		lower := now.Truncate(interval)
		upper := lower.Add(interval)
		lowerDelta := val.Sub(lower)
		upperDelta := upper.Sub(val)
		if lowerDelta < upperDelta {
			assert.Equal(t, val, lower)
		} else {
			assert.Equal(t, val, upper)
		}
		fmt.Printf("now: %v rounded: %v\n", now, val)
		time.Sleep(25 * time.Millisecond)
	}
}
