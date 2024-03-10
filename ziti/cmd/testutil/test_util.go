package testutil

import (
	"fmt"
	"io"
	"os"
	"time"
)

// CaptureOutput hot-swaps os.Stdout in order to redirect all output to a memory buffer. Where possible, do not use
// this function and instead create optional arguments/configuration to redirect output to io.Writer instances. This
// should only be used for functionality that we do not control. Many instances of its usage are unnecessary and should
// be remedied with the aforementioned solution where possible.
func CaptureOutput(function func()) string {
	oldStdOut := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	defer func() {
		_ = r.Close()
	}()

	type readResult struct {
		out []byte
		err error
	}

	defer func() {
		os.Stdout = oldStdOut
	}()

	var output []byte
	var outputErr error

	outChan := make(chan *readResult, 1)

	// Start reading before writing, so we do not create backpressure that is never relieved in OSs with smaller buffers
	// than the resulting configuration file (i.e. Windows). Go will not yield to other routines unless there is
	// a system call. The fake os.Stdout will never yield and some code paths executed as `function()` may not
	// have syscalls.
	go func() {
		output, outputErr = io.ReadAll(r)
		outChan <- &readResult{
			output,
			outputErr,
		}
	}()

	function()

	os.Stdout = oldStdOut
	_ = w.Close()

	result := <-outChan

	if result == nil {
		panic("no output")
	}

	if result.err != nil {
		panic(result.err)
	}

	return string(result.out)
}

func GenerateRandomName(baseName string) string {
	timestamp := time.Now().UnixNano() / int64(time.Millisecond)
	return fmt.Sprintf("%s_%d", baseName, timestamp)
}
