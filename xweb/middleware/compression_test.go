package middleware

import (
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func Test_getSupportedAcceptEncoding(t *testing.T) {

	t.Run("returns HttpEncodingIdentity if accept encodings are not specified", func(t *testing.T) {
		req := require.New(t)
		r := &http.Request{
			Header: map[string][]string{},
		}

		encoding := getSupportedAcceptEncoding(r)

		req.Equal(HttpEncodingIdentity, encoding)
	})

	t.Run("returns HttpEncodingIdentity if accept encodings are not supported, well formatted", func(t *testing.T) {
		req := require.New(t)
		r := &http.Request{
			Header: map[string][]string{
				HttpHeaderAcceptEncoding: {"abc,one;q=0,two,three"},
			},
		}

		encoding := getSupportedAcceptEncoding(r)

		req.Equal(HttpEncodingIdentity, encoding)
	})

	t.Run("returns HttpEncodingIdentity if accept encodings are not supported, not well formatted", func(t *testing.T) {
		req := require.New(t)
		r := &http.Request{
			Header: map[string][]string{
				HttpHeaderAcceptEncoding: {"a,b;;;;;q="},
			},
		}

		encoding := getSupportedAcceptEncoding(r)

		req.Equal(HttpEncodingIdentity, encoding)
	})

	t.Run("returns HttpEncodingIdentity if accept encodings has gzip, not well formatted", func(t *testing.T) {
		req := require.New(t)
		r := &http.Request{
			Header: map[string][]string{
				HttpHeaderAcceptEncoding: {"a,b;;;;;q=,gzip"},
			},
		}

		encoding := getSupportedAcceptEncoding(r)

		req.Equal(HttpEncodingGzip, encoding)
	})

	t.Run("returns HttpEncodingGzip if supplied as: gzip", func(t *testing.T) {
		req := require.New(t)
		r := &http.Request{
			Header: map[string][]string{
				HttpHeaderAcceptEncoding: {string(HttpEncodingGzip)},
			},
		}

		encoding := getSupportedAcceptEncoding(r)

		req.Equal(HttpEncodingGzip, encoding)
	})

	t.Run("returns HttpEncodingIdentity if supplied as: gzip, q>1", func(t *testing.T) {
		req := require.New(t)
		r := &http.Request{
			Header: map[string][]string{
				HttpHeaderAcceptEncoding: {string(HttpEncodingGzip) + ";q=1.1"},
			},
		}

		encoding := getSupportedAcceptEncoding(r)

		req.Equal(HttpEncodingIdentity, encoding)
	})

	t.Run("returns HttpEncodingIdentity if supplied as: gzip, q<0", func(t *testing.T) {
		req := require.New(t)
		r := &http.Request{
			Header: map[string][]string{
				HttpHeaderAcceptEncoding: {string(HttpEncodingGzip) + ";q=-0.1"},
			},
		}

		encoding := getSupportedAcceptEncoding(r)

		req.Equal(HttpEncodingIdentity, encoding)
	})

	t.Run("returns HttpEncodingIdentity if supplied as: gzip, non-float q", func(t *testing.T) {
		req := require.New(t)
		r := &http.Request{
			Header: map[string][]string{
				HttpHeaderAcceptEncoding: {string(HttpEncodingGzip) + ";q=abc"},
			},
		}

		encoding := getSupportedAcceptEncoding(r)

		req.Equal(HttpEncodingIdentity, encoding)
	})

	t.Run("returns HttpEncodingIdentity if supplied as: gzip, q is empty", func(t *testing.T) {
		req := require.New(t)
		r := &http.Request{
			Header: map[string][]string{
				HttpHeaderAcceptEncoding: {string(HttpEncodingGzip) + ";q="},
			},
		}

		encoding := getSupportedAcceptEncoding(r)

		req.Equal(HttpEncodingIdentity, encoding)
	})

	t.Run("returns HttpEncodingBr if supplied as: br/gzip, multiple headers, no q factors", func(t *testing.T) {
		req := require.New(t)
		r := &http.Request{
			Header: map[string][]string{
				HttpHeaderAcceptEncoding: {string(HttpEncodingBr), string(HttpEncodingGzip)},
			},
		}

		encoding := getSupportedAcceptEncoding(r)

		req.Equal(HttpEncodingBr, encoding)
	})

	t.Run("returns HttpEncodingBr if supplied as: br/gzip, one header, no q factors", func(t *testing.T) {
		req := require.New(t)
		r := &http.Request{
			Header: map[string][]string{
				HttpHeaderAcceptEncoding: {string(HttpEncodingBr) + "," + string(HttpEncodingGzip)},
			},
		}

		encoding := getSupportedAcceptEncoding(r)

		req.Equal(HttpEncodingBr, encoding)
	})

	t.Run("returns HttpEncodingBr if supplied as: br/gzip/deflate, multiple mixed header, no q factors", func(t *testing.T) {
		req := require.New(t)
		r := &http.Request{
			Header: map[string][]string{
				HttpHeaderAcceptEncoding: {
					string(HttpEncodingBr),
					string(HttpEncodingDeflate) + "," + string(HttpEncodingGzip)},
			},
		}

		encoding := getSupportedAcceptEncoding(r)

		req.Equal(HttpEncodingBr, encoding)
	})

	t.Run("returns HttpEncodingBr if supplied as: br/gzip/deflate, multiple mixed header, q factors, last header q=1 explicit", func(t *testing.T) {
		req := require.New(t)
		r := &http.Request{
			Header: map[string][]string{
				HttpHeaderAcceptEncoding: {
					string(HttpEncodingDeflate) + ";q=0.5" + "," + string(HttpEncodingGzip) + ";q=0.2",
					string(HttpEncodingBr) + ";q=1",
				},
			},
		}

		encoding := getSupportedAcceptEncoding(r)

		req.Equal(HttpEncodingBr, encoding)
	})

	t.Run("returns HttpEncodingBr if supplied as: br/gzip/deflate, multiple mixed header, q factors, last header q=1 implicit", func(t *testing.T) {
		req := require.New(t)
		r := &http.Request{
			Header: map[string][]string{
				HttpHeaderAcceptEncoding: {
					string(HttpEncodingDeflate) + ";q=0.5" + "," + string(HttpEncodingBr) + ";q=0.2",
					string(HttpEncodingGzip), //implicit q=1
				},
			},
		}

		encoding := getSupportedAcceptEncoding(r)

		req.Equal(HttpEncodingGzip, encoding)
	})

	t.Run("returns HttpEncodingBr if supplied as: br/gzip/deflate, unsupported encodings, multiple mixed header, q factors, middle header q=1 implicit", func(t *testing.T) {
		req := require.New(t)
		r := &http.Request{
			Header: map[string][]string{
				HttpHeaderAcceptEncoding: {
					"text/html;q=1",
					string(HttpEncodingDeflate) + ";q=0.9" + "," + string(HttpEncodingBr) + ";q=0.99",
					string(HttpEncodingGzip), //implicit q=1
					"text/xml",
					"ambulance;q=1,quiver;q=1,doctor",
				},
			},
		}

		encoding := getSupportedAcceptEncoding(r)

		req.Equal(HttpEncodingGzip, encoding)
	})

	t.Run("returns HttpEncodingBr if supplied as: br/gzip/deflate, unsupported encodings, multiple mixed header, q factors, middle header q=1 explicit", func(t *testing.T) {
		req := require.New(t)
		r := &http.Request{
			Header: map[string][]string{
				HttpHeaderAcceptEncoding: {
					"text/html;q=1",
					string(HttpEncodingDeflate) + ";q=0.9" + "," + string(HttpEncodingBr) + ";q=0.99",
					string(HttpEncodingGzip) + ";q=1,random;q=1,weird", //implicit q=1
					"text/xml",
					"ambulance;q=1,quiver;q=1,doctor",
				},
			},
		}

		encoding := getSupportedAcceptEncoding(r)

		req.Equal(HttpEncodingGzip, encoding)
	})

	// gzip positive tests

	t.Run("returns HttpEncodingGzip if supplied as: gzip;q=0", func(t *testing.T) {
		req := require.New(t)
		r := &http.Request{
			Header: map[string][]string{
				HttpHeaderAcceptEncoding: {string(HttpEncodingGzip) + ";q=0"},
			},
		}

		encoding := getSupportedAcceptEncoding(r)

		req.Equal(HttpEncodingGzip, encoding)
	})

	t.Run("returns HttpEncodingGzip if supplied as: gzip;q=1", func(t *testing.T) {
		req := require.New(t)
		r := &http.Request{
			Header: map[string][]string{
				HttpHeaderAcceptEncoding: {string(HttpEncodingGzip) + ";q=1"},
			},
		}

		encoding := getSupportedAcceptEncoding(r)

		req.Equal(HttpEncodingGzip, encoding)
	})

	t.Run("returns HttpEncodingGzip if supplied as: gzip;q=0.5", func(t *testing.T) {
		req := require.New(t)
		r := &http.Request{
			Header: map[string][]string{
				HttpHeaderAcceptEncoding: {string(HttpEncodingGzip) + ";q=1"},
			},
		}

		encoding := getSupportedAcceptEncoding(r)

		req.Equal(HttpEncodingGzip, encoding)
	})

	// br - positive tests

	t.Run("returns HttpEncodingBr if supplied as: br;q=0", func(t *testing.T) {
		req := require.New(t)
		r := &http.Request{
			Header: map[string][]string{
				HttpHeaderAcceptEncoding: {string(HttpEncodingBr) + ";q=0"},
			},
		}

		encoding := getSupportedAcceptEncoding(r)

		req.Equal(HttpEncodingBr, encoding)
	})

	t.Run("returns HttpEncodingBr if supplied as: br;q=1", func(t *testing.T) {
		req := require.New(t)
		r := &http.Request{
			Header: map[string][]string{
				HttpHeaderAcceptEncoding: {string(HttpEncodingBr) + ";q=1"},
			},
		}

		encoding := getSupportedAcceptEncoding(r)

		req.Equal(HttpEncodingBr, encoding)
	})

	t.Run("returns HttpEncodingBr if supplied as: br;q=0.5", func(t *testing.T) {
		req := require.New(t)
		r := &http.Request{
			Header: map[string][]string{
				HttpHeaderAcceptEncoding: {string(HttpEncodingBr) + ";q=1"},
			},
		}

		encoding := getSupportedAcceptEncoding(r)

		req.Equal(HttpEncodingBr, encoding)
	})

	// deflate - positive tests

	t.Run("returns HttpEncodingDeflate if supplied as: deflate;q=0", func(t *testing.T) {
		req := require.New(t)
		r := &http.Request{
			Header: map[string][]string{
				HttpHeaderAcceptEncoding: {string(HttpEncodingDeflate) + ";q=0"},
			},
		}

		encoding := getSupportedAcceptEncoding(r)

		req.Equal(HttpEncodingDeflate, encoding)
	})

	t.Run("returns HttpEncodingDeflate if supplied as: deflate;q=1", func(t *testing.T) {
		req := require.New(t)
		r := &http.Request{
			Header: map[string][]string{
				HttpHeaderAcceptEncoding: {string(HttpEncodingDeflate) + ";q=1"},
			},
		}

		encoding := getSupportedAcceptEncoding(r)

		req.Equal(HttpEncodingDeflate, encoding)
	})

	t.Run("returns HttpEncodingDeflate if supplied as: deflate;q=0.5", func(t *testing.T) {
		req := require.New(t)
		r := &http.Request{
			Header: map[string][]string{
				HttpHeaderAcceptEncoding: {string(HttpEncodingDeflate) + ";q=1"},
			},
		}

		encoding := getSupportedAcceptEncoding(r)

		req.Equal(HttpEncodingDeflate, encoding)
	})
}
