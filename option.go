package httpstub

type config struct {
	useTLS                              bool
	cacert, cert, key                   []byte
	clientCacert, clientCert, clientKey []byte
}

type Option func(*config) error

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
