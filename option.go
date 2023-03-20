package httpstub

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

type config struct {
	useTLS                              bool
	cacert, cert, key                   []byte
	clientCacert, clientCert, clientKey []byte
	openApi3Doc                         *openapi3.T
	skipValidateRequest                 bool
	skipValidateResponse                bool
}

type Option func(*config) error

// OpenApi3 sets OpenAPI Document using file path.
func OpenApi3(l string) Option {
	return func(c *config) error {
		ctx := context.Background()
		loader := openapi3.NewLoader()
		var doc *openapi3.T
		switch {
		case strings.HasPrefix(l, "https://") || strings.HasPrefix(l, "http://"):
			u, err := url.Parse(l)
			if err != nil {
				return err
			}
			doc, err = loader.LoadFromURI(u)
			if err != nil {
				return err
			}
		default:
			b, err := os.ReadFile(l)
			if err != nil {
				return err
			}
			doc, err = loader.LoadFromData(b)
			if err != nil {
				return err
			}
		}
		if err := doc.Validate(ctx); err != nil {
			return fmt.Errorf("openapi3 document validation error: %w", err)
		}
		c.openApi3Doc = doc
		return nil
	}
}

// OpenApi3FromData sets OpenAPI Document from bytes
func OpenApi3FromData(b []byte) Option {
	return func(c *config) error {
		ctx := context.Background()
		loader := openapi3.NewLoader()
		doc, err := loader.LoadFromData(b)
		if err != nil {
			return err
		}
		if err := doc.Validate(ctx); err != nil {
			return fmt.Errorf("openapi3 document validation error: %w", err)
		}
		c.openApi3Doc = doc
		return nil
	}
}

// SkipValidateRequest sets whether to skip validation of HTTP request with OpenAPI Document.
func SkipValidateRequest(skip bool) Option {
	return func(c *config) error {
		c.skipValidateRequest = skip
		return nil
	}
}

// SkipValidateResponse sets whether to skip validation of HTTP response with OpenAPI Document.
func SkipValidateResponse(skip bool) Option {
	return func(c *config) error {
		c.skipValidateResponse = skip
		return nil
	}
}

// UseTLS enable TLS
func UseTLS() Option {
	return func(c *config) error {
		c.useTLS = true
		return nil
	}
}

// Certificates set certificates ( cert, key )
func Certificates(cert, key []byte) Option {
	return func(c *config) error {
		c.cert = cert
		c.key = key
		return nil
	}
}

// CACert set CA
func CACert(cacert []byte) Option {
	return func(c *config) error {
		c.cacert = cacert
		return nil
	}
}

// UseTLSWithCertificates enable TLS with certificates ( cert, key )
func UseTLSWithCertificates(cert, key []byte) Option {
	return func(c *config) error {
		c.useTLS = true
		c.cert = cert
		c.key = key
		return nil
	}
}

// ClientCertificates set client certificates ( cert, key )
func ClientCertificates(cert, key []byte) Option {
	return func(c *config) error {
		c.clientCert = cert
		c.clientKey = key
		return nil
	}
}

// ClientCACert set client CA
func ClientCACert(cacert []byte) Option {
	return func(c *config) error {
		c.clientCacert = cacert
		return nil
	}
}
