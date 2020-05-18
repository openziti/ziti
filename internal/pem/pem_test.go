package pem

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func Test_firstCertBlock(t *testing.T) {
	t.Run("returns nil on invalid PEM content", func(t *testing.T) {
		ret := firstCertBlock([]byte("123456790123456790123456790123456790123456790123456790123456790123456790"))
		require.New(t).Nil(ret)
	})
}
