package handler_ctrl

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

type testTimeoutErr struct{}

func (t testTimeoutErr) Error() string {
	return "test"
}

func (t testTimeoutErr) Timeout() bool {
	return true
}

func (t testTimeoutErr) Temporary() bool {
	return true
}

func Test_TimeoutCheck(t *testing.T) {
	err := testTimeoutErr{}
	req := assert.New(t)
	req.True(isNetworkTimeout(err))

	wrapped := fmt.Errorf("there was an error (%w)", err)
	req.True(isNetworkTimeout(wrapped))
}
