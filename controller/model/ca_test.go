package model

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"net/url"
	"testing"

	"github.com/openziti/ziti/v2/controller/db"
	"github.com/stretchr/testify/require"
)

// mustParseUri parses a raw URI for SAN URI test certificates.
func mustParseUri(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	require.New(t).NoError(err)
	return u
}

// getExternalIdNoPanic invokes Ca.GetExternalId, recovering any panic so a single
// crashing configuration cannot abort sibling subtests. It reports whether the call
// panicked in addition to its normal return values.
func getExternalIdNoPanic(ca *Ca, cert *x509.Certificate) (result string, err error, panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			panicked = true
		}
	}()
	result, err = ca.GetExternalId(cert)
	return result, err, panicked
}

// Test_Ca_GetExternalId encodes the expected behavior from the issue matrix:
//   - An unsupported or incomplete externalIdClaim never panics; it returns an error.
//   - A claim that matches but resolves to an empty string returns an error rather than
//     a silently-accepted empty externalId.
//   - A valid configuration returns the extracted value.
func Test_Ca_GetExternalId(t *testing.T) {
	const sanUri = "acme:tenant:042"
	const spiffeId = "spiffe://acme/tenant/042"
	const commonName = "acme:tenant:042"
	const email = "tenant042@acme.example"

	newCert := func(t *testing.T) *x509.Certificate {
		return &x509.Certificate{
			Subject:        pkix.Name{CommonName: commonName},
			URIs:           []*url.URL{mustParseUri(t, sanUri), mustParseUri(t, spiffeId)},
			EmailAddresses: []string{email},
		}
	}

	caWith := func(claim *ExternalIdClaim) *Ca {
		return &Ca{ExternalIdClaim: claim}
	}

	t.Run("does not panic and errors on unsupported matcher for SAN URI", func(t *testing.T) {
		req := require.New(t)
		for _, matcher := range []string{db.ExternalIdClaimMatcherPrefix, db.ExternalIdClaimMatcherSuffix} {
			ca := caWith(&ExternalIdClaim{
				Location:        db.ExternalIdClaimLocSanUri,
				Matcher:         matcher,
				MatcherCriteria: "acme:tenant:",
				Parser:          db.ExternalIdClaimParserNone,
			})
			_, err, panicked := getExternalIdNoPanic(ca, newCert(t))
			req.False(panicked, "matcher %s on SAN URI should not panic", matcher)
			req.Error(err, "matcher %s on SAN URI should return an error", matcher)
		}
	})

	t.Run("does not panic and errors on missing matcher criteria", func(t *testing.T) {
		req := require.New(t)
		cases := []*ExternalIdClaim{
			{Location: db.ExternalIdClaimLocSanUri, Matcher: db.ExternalIdClaimMatcherScheme, MatcherCriteria: "", Parser: db.ExternalIdClaimParserNone},
			{Location: db.ExternalIdClaimLocCommonName, Matcher: db.ExternalIdClaimMatcherPrefix, MatcherCriteria: "", Parser: db.ExternalIdClaimParserNone},
			{Location: db.ExternalIdClaimLocSanEmail, Matcher: db.ExternalIdClaimMatcherSuffix, MatcherCriteria: "", Parser: db.ExternalIdClaimParserNone},
		}
		for _, claim := range cases {
			_, err, panicked := getExternalIdNoPanic(caWith(claim), newCert(t))
			req.False(panicked, "claim %+v should not panic", claim)
			req.Error(err, "claim %+v should return an error", claim)
		}
	})

	t.Run("does not panic and errors on missing parser criteria for split", func(t *testing.T) {
		req := require.New(t)
		ca := caWith(&ExternalIdClaim{
			Location:       db.ExternalIdClaimLocCommonName,
			Matcher:        db.ExternalIdClaimMatcherAll,
			Parser:         db.ExternalIdClaimParserSplit,
			ParserCriteria: "",
		})
		_, err, panicked := getExternalIdNoPanic(ca, newCert(t))
		req.False(panicked, "split parser with empty criteria should not panic")
		req.Error(err, "split parser with empty criteria should return an error")
	})

	t.Run("does not panic and errors on negative index", func(t *testing.T) {
		req := require.New(t)
		ca := caWith(&ExternalIdClaim{
			Location: db.ExternalIdClaimLocCommonName,
			Matcher:  db.ExternalIdClaimMatcherAll,
			Parser:   db.ExternalIdClaimParserNone,
			Index:    -1,
		})
		_, err, panicked := getExternalIdNoPanic(ca, newCert(t))
		req.False(panicked, "negative index should not panic")
		req.Error(err, "negative index should return an error")
	})

	t.Run("errors when a matched claim resolves to empty", func(t *testing.T) {
		req := require.New(t)
		// PREFIX trims the entire common name, leaving an empty claim.
		ca := caWith(&ExternalIdClaim{
			Location:        db.ExternalIdClaimLocCommonName,
			Matcher:         db.ExternalIdClaimMatcherPrefix,
			MatcherCriteria: commonName,
			Parser:          db.ExternalIdClaimParserNone,
		})
		result, err, panicked := getExternalIdNoPanic(ca, newCert(t))
		req.False(panicked)
		req.Error(err, "a matched-but-empty claim must not be accepted as an empty externalId")
		req.Empty(result)
	})

	t.Run("returns the value for a valid SAN URI scheme claim", func(t *testing.T) {
		req := require.New(t)
		ca := caWith(&ExternalIdClaim{
			Location:        db.ExternalIdClaimLocSanUri,
			Matcher:         db.ExternalIdClaimMatcherScheme,
			MatcherCriteria: "spiffe",
			Parser:          db.ExternalIdClaimParserNone,
		})
		result, err, panicked := getExternalIdNoPanic(ca, newCert(t))
		req.False(panicked)
		req.NoError(err)
		req.Equal(spiffeId, result)
	})

	t.Run("returns the value for a valid common name all claim", func(t *testing.T) {
		req := require.New(t)
		ca := caWith(&ExternalIdClaim{
			Location: db.ExternalIdClaimLocCommonName,
			Matcher:  db.ExternalIdClaimMatcherAll,
			Parser:   db.ExternalIdClaimParserNone,
		})
		result, err, panicked := getExternalIdNoPanic(ca, newCert(t))
		req.False(panicked)
		req.NoError(err)
		req.Equal(commonName, result)
	})

	t.Run("treats a claim with an empty location as no claim", func(t *testing.T) {
		req := require.New(t)
		// An externalIdClaim with no location is unconfigured (e.g. the empty bucket left by an
		// older CLI's empty {} patch). It must read as "no claim" -> fall back to fingerprint,
		// not error, so existing CAs keep enrolling after upgrade.
		ca := caWith(&ExternalIdClaim{})
		result, err, panicked := getExternalIdNoPanic(ca, newCert(t))
		req.False(panicked)
		req.NoError(err)
		req.Empty(result)
	})
}

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
