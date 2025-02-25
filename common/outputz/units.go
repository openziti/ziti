package outputz

import "fmt"

var units = []string{"B", "K", "M", "G", "T", "P"}

func FormatBytes(val uint64) string {
	var i int
	var target uint64
	for i = range units {
		target = 1 << uint(10*(i+1))
		if val < target {
			break
		}
	}
	if i > 0 {
		return fmt.Sprintf("%0.2f%s", float64(val)/(float64(target)/1024), units[i])
	}
	return fmt.Sprintf("%dB", val)
}
