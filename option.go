package httpstub

type config struct {
	useTLS            bool
	cacert, cert, key []byte
}

type Option func(*config) error

func UseTLS() Option {
	return func(c *config) error {
		c.useTLS = true
		return nil
	}
}

func UseTLSWithCertificates(cacert, cert, key []byte) Option {
	return func(c *config) error {
		c.useTLS = true
		c.cacert = cacert
		c.cert = cert
		c.key = key
		return nil
	}
}
