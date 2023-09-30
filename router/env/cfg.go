package env

import "time"

func init() {
	IntervalSize = time.Minute
}

var IntervalSize time.Duration
