package edge

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestInputParser(t *testing.T) {
	inputProtocol := "tcp"
	inputAddress := "192.168.1.1"
	inputPort := 8080
	defaultAddress := "127.0.0.1"
	defaultProtocol := "tcp\", \"udp"
	errorPort := 0
	errorProtOrAddr := ""

	t.Run("TestFullInput", func(t *testing.T) {
		inputValue := fmt.Sprintf("%s:%s:%d", inputProtocol, inputAddress, inputPort)
		actProtocol, actAddress, actPort, err := ParseInput(inputValue)
		assert.Equal(t, inputProtocol, actProtocol)
		assert.Equal(t, inputAddress, actAddress)
		assert.Equal(t, inputPort, actPort)
		assert.Nil(t, err)
	})
	t.Run("TestNoAddr", func(t *testing.T) {
		inputValue := fmt.Sprintf("%s:%d", inputProtocol, inputPort)
		actProtocol, actAddress, actPort, err := ParseInput(inputValue)
		assert.Equal(t, inputProtocol, actProtocol)
		assert.Equal(t, defaultAddress, actAddress)
		assert.Equal(t, inputPort, actPort)
		assert.Nil(t, err)
	})
	t.Run("TestNoProt", func(t *testing.T) {
		inputValue := fmt.Sprintf("%s:%d", inputAddress, inputPort)
		actProtocol, actAddress, actPort, err := ParseInput(inputValue)
		assert.Equal(t, defaultProtocol, actProtocol)
		assert.Equal(t, inputAddress, actAddress)
		assert.Equal(t, inputPort, actPort)
		assert.Nil(t, err)
	})
	t.Run("TestNoAddrNoProt", func(t *testing.T) {
		inputValue := fmt.Sprintf("%d", inputPort)
		actProtocol, actAddress, actPort, err := ParseInput(inputValue)
		assert.Equal(t, defaultProtocol, actProtocol)
		assert.Equal(t, defaultAddress, actAddress)
		assert.Equal(t, inputPort, actPort)
		assert.Nil(t, err)
	})
	t.Run("TestProtAddrNoPortFails", func(t *testing.T) {
		inputValue := fmt.Sprintf("%s:%s", inputProtocol, inputAddress)
		actProtocol, actAddress, actPort, err := ParseInput(inputValue)
		assert.Equal(t, errorProtOrAddr, actProtocol)
		assert.Equal(t, errorProtOrAddr, actAddress)
		assert.Equal(t, errorPort, actPort)
		assert.NotNil(t, err)
	})
	t.Run("TestOnlyProtFails", func(t *testing.T) {
		inputValue := fmt.Sprintf("%s", inputProtocol)
		actProtocol, actAddress, actPort, err := ParseInput(inputValue)
		assert.Equal(t, errorProtOrAddr, actProtocol)
		assert.Equal(t, errorProtOrAddr, actAddress)
		assert.Equal(t, errorPort, actPort)
		assert.NotNil(t, err)
	})
	t.Run("TestOnlyAddrFails", func(t *testing.T) {
		inputValue := fmt.Sprintf("%s", inputProtocol)
		actProtocol, actAddress, actPort, err := ParseInput(inputValue)
		assert.Equal(t, errorProtOrAddr, actProtocol)
		assert.Equal(t, errorProtOrAddr, actAddress)
		assert.Equal(t, errorPort, actPort)
		assert.NotNil(t, err)
	})
}
