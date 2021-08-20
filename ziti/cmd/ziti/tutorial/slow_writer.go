package tutorial

import (
	"io"
	"os"
	"time"
)

func NewSlowWriter(newlinePause time.Duration) io.Writer {
	return &slowWriter{
		delay: newlinePause,
	}
}

type slowWriter struct {
	delay time.Duration
}

func (self *slowWriter) Write(p []byte) (n int, err error) {
	var buf []byte
	written := 0
	for _, b := range p {
		buf = append(buf, b)
		if b == '\n' {
			time.Sleep(self.delay)
			n, err := os.Stdout.Write(buf)
			buf = buf[0:0]
			written += n
			if err != nil {
				return written, err
			}
		}
	}

	if len(buf) > 0 {
		n, err := os.Stdout.Write(buf)
		written += n
		if err != nil {
			return written, err
		}
	}
	return written, nil
}
