package httpstub

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"
	vconfig "github.com/pb33f/libopenapi-validator/config"
	"github.com/pb33f/libopenapi/datamodel"
)

type config struct {
	useTLS                              bool
	cacert, cert, key                   []byte
	clientCacert, clientCert, clientKey []byte
	openAPI3Doc                         libopenapi.Document
	openAPI3Validator                   validator.Validator
	skipValidateRequest                 bool
	skipValidateResponse                bool
	skipCircularReferenceCheck          bool
	addr                                string
	basePath                            string
	seed                                int64
	responseMode                        ResponseMode
}

type Option func(*config) error

// OpenApi3 sets OpenAPI Document using file path.
func OpenApi3(l string) Option {
	return func(c *config) error {
		var doc libopenapi.Document
		dc := &datamodel.DocumentConfiguration{
			AllowFileReferences:        true,
			AllowRemoteReferences:      true,
			SkipCircularReferenceCheck: c.skipCircularReferenceCheck,
		}
		switch {
		case strings.HasPrefix(l, "https://") || strings.HasPrefix(l, "http://"):
			// Add URL validation
			if _, err := url.Parse(l); err != nil {
				return fmt.Errorf("invalid URL: %w", err)
			}

			// #nosec G107 - URL is validated
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
		v, errs := validator.NewValidator(doc, vconfig.WithSchemaCache(nil))
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
		c.openAPI3Doc = doc
		c.openAPI3Validator = v
		return nil
	}
}

// OpenApi3FromData sets OpenAPI Document from bytes.
func OpenApi3FromData(b []byte) Option {
	return func(c *config) error {
		dc := &datamodel.DocumentConfiguration{
			AllowFileReferences:        true,
			AllowRemoteReferences:      true,
			SkipCircularReferenceCheck: c.skipCircularReferenceCheck,
		}
		doc, err := libopenapi.NewDocumentWithConfiguration(b, dc)
		if err != nil {
			return err
		}
		v, errs := validator.NewValidator(doc, vconfig.WithSchemaCache(nil))
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
		c.openAPI3Doc = doc
		c.openAPI3Validator = v
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

// SkipCircularReferenceCheck sets whether to skip circular reference check in OpenAPI Document.
func SkipCircularReferenceCheck(skip bool) Option {
	return func(c *config) error {
		c.skipCircularReferenceCheck = skip
		return nil
	}
}

// UseTLS enable TLS.
func UseTLS() Option {
	return func(c *config) error {
		c.useTLS = true
		return nil
	}
}

// Certificates set certificates ( cert, key ).
func Certificates(cert, key []byte) Option {
	return func(c *config) error {
		c.cert = cert
		c.key = key
		return nil
	}
}

// CACert set CA.
func CACert(cacert []byte) Option {
	return func(c *config) error {
		c.cacert = cacert
		return nil
	}
}

// UseTLSWithCertificates enable TLS with certificates ( cert, key ).
func UseTLSWithCertificates(cert, key []byte) Option {
	return func(c *config) error {
		c.useTLS = true
		c.cert = cert
		c.key = key
		return nil
	}
}

// ClientCertificates set client certificates ( cert, key ).
func ClientCertificates(cert, key []byte) Option {
	return func(c *config) error {
		c.clientCert = cert
		c.clientKey = key
		return nil
	}
}

// ClientCACert set client CA.
func ClientCACert(cacert []byte) Option {
	return func(c *config) error {
		c.clientCacert = cacert
		return nil
	}
}

// Addr set server address.
func Addr(addr string) Option {
	return func(c *config) error {
		c.addr = addr
		return nil
	}
}

// BasePath set base URL path prefix.
func BasePath(basePath string) Option {
	return func(c *config) error {
		if !strings.HasPrefix(basePath, "/") {
			return errors.New("basePath must start with '/'")
		}
		if strings.HasSuffix(basePath, "/") {
			return errors.New("basePath must not end with '/'")
		}
		c.basePath = basePath
		return nil
	}
}

// Seed sets a seed for deterministic mock data generation for the entire router.
func Seed(seed int64) Option {
	return func(c *config) error {
		c.seed = seed
		return nil
	}
}

// WithResponseMode sets the response mode for ResponseDynamic.
// - AlwaysGenerate: Always generate from schema (ignore examples) - default
// - ExamplesOnly: Use only explicit examples (error if not found)
// - PreferExamples: Prefer examples, fallback to schema generation
func WithResponseMode(mode ResponseMode) Option {
	return func(c *config) error {
		c.responseMode = mode
		return nil
	}
}
