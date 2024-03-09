package httpstub

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"
	"github.com/pb33f/libopenapi/datamodel"
)

const (
	openAPIVersion3 = 3
	openAPIVersion2 = 2
)

type config struct {
	useTLS                              bool
	cacert, cert, key                   []byte
	clientCacert, clientCert, clientKey []byte
	openAPIVersion                      int
	openAPIDoc                          *libopenapi.Document
	openAPIValidator                    *validator.Validator
	skipValidateRequest                 bool
	skipValidateResponse                bool
}

type Option func(*config) error

// OpenAPI3 sets OpenAPI Document v3 using path.
func OpenAPI3(l string) Option {
	return func(c *config) error {
		opt := openAPI(l)
		if err := opt(c); err != nil {
			return err
		}
		c.openAPIVersion = openAPIVersion3
		return nil
	}
}

// OpenAPI2 sets OpenAPI Document v2 using path.
func OpenAPI2(l string) Option {
	return func(c *config) error {
		opt := openAPI(l)
		if err := opt(c); err != nil {
			return err
		}
		c.openAPIVersion = openAPIVersion2
		return nil
	}
}

// OpenApi3 sets OpenAPI Document v3 using path.
// Deprecated: Use OpenAPI3 instead.
func OpenApi3(l string) Option {
	return OpenAPI3(l)
}

// openAPI sets OpenAPI Document using path.
func openAPI(l string) Option {
	return func(c *config) error {
		var doc libopenapi.Document
		dc := &datamodel.DocumentConfiguration{
			AllowFileReferences:   true,
			AllowRemoteReferences: true,
		}
		switch {
		case strings.HasPrefix(l, "https://") || strings.HasPrefix(l, "http://"):
			res, err := http.Get(l)
			if err != nil {
				return err
			}
			defer res.Body.Close()
			b, err := io.ReadAll(res.Body)
			if err != nil {
				return err
			}
			doc, err = libopenapi.NewDocumentWithConfiguration(b, dc)
			if err != nil {
				return err
			}
		default:
			b, err := os.ReadFile(l)
			if err != nil {
				return err
			}
			doc, err = libopenapi.NewDocumentWithConfiguration(b, dc)
			if err != nil {
				return err
			}
		}
		v, errs := validator.NewValidator(doc)
		if len(errs) > 0 {
			return errors.Join(errs...)
		}
		if _, errs := v.ValidateDocument(); len(errs) > 0 {
			var err error
			for _, e := range errs {
				err = errors.Join(err, e)
			}
			return err
		}
		c.openAPIDoc = &doc
		c.openAPIValidator = &v
		return nil
	}
}

// OpenAPI3FromData sets OpenAPI Document v3 from bytes
func OpenAPI3FromData(b []byte) Option {
	return func(c *config) error {
		opt := openAPIFromData(b)
		if err := opt(c); err != nil {
			return err
		}
		c.openAPIVersion = openAPIVersion3
		return nil
	}
}

// OpenAPI2FromData sets OpenAPI Document v2 from bytes
func OpenAPI2FromData(b []byte) Option {
	return func(c *config) error {
		opt := openAPIFromData(b)
		if err := opt(c); err != nil {
			return err
		}
		c.openAPIVersion = openAPIVersion2
		return nil
	}
}

// OpenApi3FromData sets OpenAPI Document v3 from bytes
// Deprecated: Use OpenAPI3FromData instead.
func OpenApi3FromData(b []byte) Option {
	return OpenAPI3FromData(b)
}

// openAPIFromData sets OpenAPI Document from bytes
func openAPIFromData(b []byte) Option {
	return func(c *config) error {
		doc, err := libopenapi.NewDocument(b)
		if err != nil {
			return err
		}
		v, errs := validator.NewValidator(doc)
		if len(errs) > 0 {
			return errors.Join(errs...)
		}
		if _, errs := v.ValidateDocument(); len(errs) > 0 {
			var err error
			for _, e := range errs {
				err = errors.Join(err, e)
			}
			return err
		}
		c.openAPIDoc = &doc
		c.openAPIValidator = &v
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
