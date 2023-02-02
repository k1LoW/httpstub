package httpstub

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
)

var _ http.Handler = (*Router)(nil)

type Router struct {
	matchers          []*matcher
	server            *httptest.Server
	middlewares       middlewareFuncs
	requests          []*http.Request
	t                 *testing.T
	useTLS            bool
	cacert, cert, key []byte
	mu                sync.RWMutex
}

type matcher struct {
	matchFuncs  []matchFunc
	handler     http.HandlerFunc
	middlewares middlewareFuncs
	requests    []*http.Request
	mu          sync.RWMutex
}

type matchFunc func(r *http.Request) bool
type middlewareFunc func(next http.HandlerFunc) http.HandlerFunc
type middlewareFuncs []middlewareFunc

func (mws middlewareFuncs) then(fn http.HandlerFunc) http.HandlerFunc {
	for i := range mws {
		fn = mws[len(mws)-1-i](fn)
	}
	return fn
}

func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rt.t.Helper()
	r2 := cloneReq(r)
	rt.mu.Lock()
	rt.requests = append(rt.requests, r2)
	rt.mu.Unlock()

	for _, m := range rt.matchers {
		match := true
		for _, fn := range m.matchFuncs {
			if !fn(r) {
				match = false
			}
		}
		if match {
			m.mu.Lock()
			m.requests = append(m.requests, r2)
			m.mu.Unlock()
			rt.mu.RLock()
			mws := append(rt.middlewares, m.middlewares...)
			rt.mu.RUnlock()
			mws.then(m.handler).ServeHTTP(w, r)
			return
		}
	}
	dump, _ := httputil.DumpRequest(r, true)
	rt.t.Errorf("httpstub error: request did not match\n---REQUEST START---\n%s\n---REQUEST END---\n", string(dump))
}

// NewRouter returns a new router with methods for stubbing.
func NewRouter(t *testing.T, opts ...Option) *Router {
	t.Helper()
	c := &config{}
	for _, opt := range opts {
		if err := opt(c); err != nil {
			t.Fatal(err)
		}
	}
	return &Router{
		t:      t,
		useTLS: c.useTLS,
		cacert: c.cacert,
		cert:   c.cert,
		key:    c.key,
	}
}

// NewServer returns a new router including *httptest.Server.
func NewServer(t *testing.T, opts ...Option) *Router {
	t.Helper()
	rt := NewRouter(t, opts...)
	_ = rt.Server()
	return rt
}

// NewTLSServer returns a new router including TLS *httptest.Server.
func NewTLSServer(t *testing.T, opts ...Option) *Router {
	t.Helper()
	rt := NewRouter(t, opts...)
	rt.useTLS = true
	_ = rt.TLSServer()
	return rt
}

// Client returns *http.Client which requests *httptest.Server.
func (rt *Router) Client() *http.Client {
	if rt.server == nil {
		rt.t.Error("server is not started yet")
		return nil
	}
	return rt.server.Client()
}

// Server returns *httptest.Server with *Router set.
func (rt *Router) Server() *httptest.Server {
	if rt.server != nil {
		return rt.server
	}
	if rt.useTLS {
		rt.server = httptest.NewUnstartedServer(rt)
		if len(rt.cert) > 0 && len(rt.key) > 0 {
			cert, err := tls.X509KeyPair(rt.cert, rt.key)
			if err != nil {
				panic(err)
			}
			existingConfig := rt.server.TLS
			if existingConfig != nil {
				rt.server.TLS = existingConfig.Clone()
			} else {
				rt.server.TLS = new(tls.Config)
			}
			rt.server.TLS.Certificates = []tls.Certificate{cert}
		}
		rt.server.StartTLS()
		if len(rt.cacert) > 0 {
			certpool, err := x509.SystemCertPool()
			if err != nil {
				// FIXME for Windows
				// ref: https://github.com/golang/go/issues/18609
				certpool = x509.NewCertPool()
			}
			if !certpool.AppendCertsFromPEM(rt.cacert) {
				panic("failed to add cacert")
			}
			client := rt.server.Client()
			client.Transport.(*http.Transport).TLSClientConfig.RootCAs = certpool
		}
	} else {
		rt.server = httptest.NewServer(rt)
	}
	client := rt.server.Client()
	tp := client.Transport.(*http.Transport)
	client.Transport = newTransport(rt.server.URL, tp)
	return rt.server
}

// TLSServer returns TLS *httptest.Server with *Router set.
func (rt *Router) TLSServer() *httptest.Server {
	rt.useTLS = true
	return rt.Server()
}

// Close shuts down *httptest.Server
func (rt *Router) Close() {
	if rt.server == nil {
		rt.t.Error("server is not started yet")
		return
	}
	rt.server.Close()
}

// Match create request matcher with matchFunc (func(r *http.Request) bool).
func (rt *Router) Match(fn func(r *http.Request) bool) *matcher {
	m := &matcher{
		matchFuncs: []matchFunc{fn},
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.matchers = append(rt.matchers, m)
	return m
}

// Match append matchFunc (func(r *http.Request) bool) to request matcher.
func (m *matcher) Match(fn func(r *http.Request) bool) *matcher {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.matchFuncs = append(m.matchFuncs, fn)
	return m
}

// Method create request matcher using method.
func (rt *Router) Method(method string) *matcher {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	fn := methodMatchFunc(method)
	m := &matcher{
		matchFuncs: []matchFunc{fn},
	}
	rt.matchers = append(rt.matchers, m)
	return m
}

// Method append matcher using method to request matcher.
func (m *matcher) Method(method string) *matcher {
	m.mu.Lock()
	defer m.mu.Unlock()
	fn := methodMatchFunc(method)
	m.matchFuncs = append(m.matchFuncs, fn)
	return m
}

// Path create request matcher using path.
func (rt *Router) Path(path string) *matcher {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	fn := pathMatchFunc(path)
	m := &matcher{
		matchFuncs: []matchFunc{fn},
	}
	rt.matchers = append(rt.matchers, m)
	return m
}

// Path append matcher using path to request matcher.
func (m *matcher) Path(path string) *matcher {
	m.mu.Lock()
	defer m.mu.Unlock()
	fn := pathMatchFunc(path)
	m.matchFuncs = append(m.matchFuncs, fn)
	return m
}

// DefaultMiddleware append default middleware.
func (rt *Router) DefaultMiddleware(mw func(next http.HandlerFunc) http.HandlerFunc) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.middlewares = append(rt.middlewares, mw)
}

// DefaultHeader append default middleware which append header.
func (rt *Router) DefaultHeader(key, value string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	mw := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add(key, value)
			next.ServeHTTP(w, r)
		}
	}
	rt.middlewares = append(rt.middlewares, mw)
}

// Middleware append middleware to matcher.
func (m *matcher) Middleware(mw func(next http.HandlerFunc) http.HandlerFunc) *matcher {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.middlewares = append(m.middlewares, mw)
	return m
}

// Header append middleware which append header to response.
func (m *matcher) Header(key, value string) *matcher {
	m.mu.Lock()
	defer m.mu.Unlock()
	mw := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add(key, value)
			next.ServeHTTP(w, r)
		}
	}
	m.middlewares = append(m.middlewares, mw)
	return m
}

// Handler set hander.
func (m *matcher) Handler(fn func(w http.ResponseWriter, r *http.Request)) {
	m.handler = http.HandlerFunc(fn)
}

// Response set hander which return response.
func (m *matcher) Response(status int, body []byte) {
	fn := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}
	m.handler = http.HandlerFunc(fn)
}

// ResponseString set hander which return response.
func (m *matcher) ResponseString(status int, body string) {
	b := []byte(body)
	m.Response(status, b)
}

// Requests returns []*http.Request received by router.
func (rt *Router) Requests() []*http.Request {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.requests
}

// Requests returns []*http.Request received by matcher.
func (m *matcher) Requests() []*http.Request {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.requests
}

func cloneReq(r *http.Request) *http.Request {
	r2 := r.Clone(r.Context())
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body))
	r2.Body = io.NopCloser(bytes.NewReader(body))
	return r2
}

func methodMatchFunc(method string) matchFunc {
	return func(r *http.Request) bool {
		return r.Method == method
	}
}

func pathMatchFunc(path string) matchFunc {
	pathRe := regexp.MustCompile(strings.Replace(path, "/*", "/.*", -1))
	return func(r *http.Request) bool {
		return pathRe.MatchString(r.URL.Path)
	}
}

type transport struct {
	URL *url.URL
	tp  *http.Transport
}

func newTransport(rawURL string, tp *http.Transport) http.RoundTripper {
	u, _ := url.Parse(rawURL)
	return &transport{
		URL: u,
		tp:  tp,
	}
}

func (t *transport) transport() http.RoundTripper {
	return t.tp
}

func (t *transport) CancelRequest(r *http.Request) {
	type canceler interface {
		CancelRequest(*http.Request)
	}
	if cr, ok := t.transport().(canceler); ok {
		cr.CancelRequest(r)
	}
}

func (t *transport) RoundTrip(r *http.Request) (*http.Response, error) {
	r.URL.Scheme = t.URL.Scheme
	r.URL.Host = t.URL.Host
	r.URL.User = t.URL.User
	r.URL.Opaque = t.URL.Opaque
	res, err := t.transport().RoundTrip(r)
	return res, err
}
