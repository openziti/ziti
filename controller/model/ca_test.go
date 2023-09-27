package model

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func Test_IdentityNameFormatter(t *testing.T) {
	const (
		valueCaName       = "myCaIsCool"
		valueCaId         = "1234567890"
		valueCommonName   = "myFirstCert"
		valueIdentityName = "laptop01"
		valueIdentityId   = "someIdHere"
	)

	symbols := map[string]string{
		FormatSymbolCaName:        valueCaName,
		FormatSymbolCaId:          valueCaId,
		FormatSymbolCommonName:    valueCommonName,
		FormatSymbolRequestedName: valueIdentityName,
		FormatSymbolIdentityId:    valueIdentityId,
	}

	formatter := NewFormatter(symbols)

	t.Run("replaces all repeating symbols", func(t *testing.T) {
		caSymbol := fmt.Sprintf("%s%s%s", formatter.sentinelStart, FormatSymbolCaName, formatter.sentinelEnd)
		threeSymbols := fmt.Sprintf("%s - %s - %s", caSymbol, caSymbol, caSymbol)

		outputName := formatter.Format(threeSymbols)
		expectedName := fmt.Sprintf("%s - %s - %s", valueCaName, valueCaName, valueCaName)

		require.New(t).Equal(expectedName, outputName)
	})

	t.Run("replaces all repeating symbols w/o spaces", func(t *testing.T) {
		caSymbol := fmt.Sprintf("%s%s%s", formatter.sentinelStart, FormatSymbolCaName, formatter.sentinelEnd)
		threeSymbols := fmt.Sprintf("%s%s%s", caSymbol, caSymbol, caSymbol)

		outputName := formatter.Format(threeSymbols)
		expectedName := fmt.Sprintf("%s%s%s", valueCaName, valueCaName, valueCaName)

		require.New(t).Equal(expectedName, outputName)
	})

	t.Run("replaces nothing when no symbols are present", func(t *testing.T) {
		input := "dude sucking at something is the first step to being sorta good at something"
		output := formatter.Format(input)

		require.New(t).Equal(input, output)
	})

	t.Run("works with empty string", func(t *testing.T) {
		input := ""
		output := formatter.Format(input)

		require.New(t).Equal(input, output)
	})

	t.Run("replaces multiple different symbols", func(t *testing.T) {
		input := ""
		expected := ""
		for symbol, value := range symbols {
			symbol = fmt.Sprintf("%s%s%s", formatter.sentinelStart, symbol, formatter.sentinelEnd)
			input = input + symbol
			expected = expected + value
		}

		output := formatter.Format(input)

		require.New(t).Equal(expected, output)
	})

	t.Run("replaces nested symbols left to right, inside out w/o collisions", func(t *testing.T) {
		// [[requestedName]requestedName] -> [laptop01requestedName]
		input := fmt.Sprintf("%s%s%s%s%s%s", formatter.sentinelStart, formatter.sentinelStart, FormatSymbolRequestedName, formatter.sentinelEnd, FormatSymbolRequestedName, formatter.sentinelEnd)
		expected := fmt.Sprintf("%s%s%s%s", formatter.sentinelStart, valueIdentityName, FormatSymbolRequestedName, formatter.sentinelEnd)

		output := formatter.Format(input)

		require.New(t).Equal(expected, output)
	})
}
