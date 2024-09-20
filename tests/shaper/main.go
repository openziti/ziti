package main

import (
	"fmt"
	"github.com/openziti/metrics"
	"github.com/openziti/transport/v2/shaper"
	"sync/atomic"
	"time"
)

type metricsWriter struct {
	m metrics.Meter
}

func (self *metricsWriter) Write(p []byte) (n int, err error) {
	l := len(p)
	self.m.Mark(int64(l))
	return l, nil
}

func printRate(rate float64, desc string) {
	units := []string{"B", "K", "M", "G", "T", "P"}
	index := 0
	for rate > 1000 {
		rate = rate / 1000
		index++
	}
	fmt.Printf("%s: %.2f%s\n", desc, rate, units[index])
}

func main() {
	r := metrics.NewRegistry("test", nil)
	meter := r.Meter("writes")
	w := &metricsWriter{m: meter}
	f := shaper.LimitWriter(w, time.Second, 500000)

	var written int64

	go func() {
		var last int64
		for {
			rate := r.Poll().Meters["writes"].M1Rate
			printRate(rate, "m1_rate")

			cur := atomic.LoadInt64(&written)
			rate = float64(cur - last)
			last = cur
			printRate(rate, "alt_rate")
			time.Sleep(time.Second)
		}
	}()

	b := make([]byte, 1500)
	for {
		n, _ := f.Write(b)
		atomic.AddInt64(&written, int64(n))
	}
}
