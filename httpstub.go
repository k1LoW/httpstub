package httpstub

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/minio/pkg/wildcard"
	"github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"
	"github.com/pb33f/libopenapi-validator/paths"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	openapijson "github.com/pb33f/libopenapi/json"
	"github.com/pb33f/libopenapi/orderedmap"
)

var (
	_ http.Handler = (*Router)(nil)
	_ TB           = (testing.TB)(nil)
)

type TB interface {
	Error(args ...any)
	Errorf(format string, args ...any)
	Fatal(args ...any)
	Fatalf(format string, args ...any)
	Helper()
}

type Router struct {
	// Set *httptest.Server.URL
	URL                                 string
	matchers                            []*matcher
	server                              *httptest.Server
	middlewares                         middlewareFuncs
	requests                            []*http.Request
	t                                   TB
	useTLS                              bool
	cacert, cert, key                   []byte
	clientCacert, clientCert, clientKey []byte
	openapi3Doc                         *libopenapi.Document
	openapi3Validator                   *validator.Validator
	skipValidateRequest                 bool
	skipValidateResponse                bool
	mu                                  sync.RWMutex
}

type matcher struct {
	matchFuncs  []matchFunc
	handler     http.HandlerFunc
	middlewares middlewareFuncs
	requests    []*http.Request
	router      *Router
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
func NewRouter(t TB, opts ...Option) *Router {
	t.Helper()
	c := &config{}
	for _, opt := range opts {
		if err := opt(c); err != nil {
			t.Fatal(err)
		}
	}
	rt := &Router{
		t:                 t,
		useTLS:            c.useTLS,
		cacert:            c.cacert,
		cert:              c.cert,
		key:               c.key,
		clientCacert:      c.clientCacert,
		clientCert:        c.clientCert,
		clientKey:         c.clientKey,
		openapi3Doc:       c.openapi3Doc,
		openapi3Validator: c.openapi3Validator,
	}
	if err := rt.setOpenApi3Vaildator(); err != nil {
		t.Fatal(err)
	}
	return rt
}

// NewServer returns a new router including *httptest.Server.
func NewServer(t TB, opts ...Option) *Router {
	t.Helper()
	rt := NewRouter(t, opts...)
	s := rt.Server()
	rt.URL = s.URL
	return rt
}

// NewTLSServer returns a new router including TLS *httptest.Server.
func NewTLSServer(t TB, opts ...Option) *Router {
	t.Helper()
	rt := NewRouter(t, opts...)
	rt.useTLS = true
	s := rt.TLSServer()
	rt.URL = s.URL
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

		// server certificates
		if rt.cert != nil && rt.key != nil {
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
		// client CA
		if rt.clientCacert != nil {
			certpool, err := x509.SystemCertPool()
			if err != nil {
				// FIXME for Windows
				// ref: https://github.com/golang/go/issues/18609
				certpool = x509.NewCertPool()
			}
			if !certpool.AppendCertsFromPEM(rt.clientCacert) {
				panic("failed to add cacert")
			}
			existingConfig := rt.server.TLS
			if existingConfig != nil {
				rt.server.TLS = existingConfig.Clone()
			} else {
				rt.server.TLS = new(tls.Config)
			}
			rt.server.TLS.ClientCAs = certpool
			rt.server.TLS.ClientAuth = tls.RequireAndVerifyClientCert
		}

		rt.server.StartTLS()

		// server CA
		if rt.cacert != nil {
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
		// client certificates
		if rt.clientCert != nil && rt.clientKey != nil {
			cert, err := tls.X509KeyPair(rt.clientCert, rt.clientKey)
			if err != nil {
				panic(err)
			}
			client := rt.server.Client()
			client.Transport.(*http.Transport).TLSClientConfig.Certificates = []tls.Certificate{cert}
		}
	} else {
		rt.server = httptest.NewServer(rt)
	}
	client := rt.server.Client()
	tp := client.Transport.(*http.Transport)
	client.Transport = newTransport(rt.server.URL, tp)
	rt.URL = rt.server.URL
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
		matchFuncs: []matchFunc{withCloneReq(fn)},
		router:     rt,
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
	m.matchFuncs = append(m.matchFuncs, withCloneReq(fn))
	return m
}

// Method create request matcher using method.
func (rt *Router) Method(method string) *matcher {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	fn := methodMatchFunc(method)
	m := &matcher{
		matchFuncs: []matchFunc{fn},
		router:     rt,
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

// Pathf create request matcher using sprintf-ed path.
func (rt *Router) Pathf(format string, a ...any) *matcher {
	return rt.Path(fmt.Sprintf(format, a...))
}

// Pathf append matcher using sprintf-ed path to request matcher.
func (m *matcher) Pathf(format string, a ...any) *matcher {
	return m.Path(fmt.Sprintf(format, a...))
}

// Query create request matcher using query.
func (rt *Router) Query(key, value string) *matcher {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	fn := queryMatchFunc(key, value)
	m := &matcher{
		matchFuncs: []matchFunc{fn},
	}
	rt.matchers = append(rt.matchers, m)
	return m
}

// Query append matcher using query to request matcher.
func (m *matcher) Query(key, value string) *matcher {
	m.mu.Lock()
	defer m.mu.Unlock()
	fn := queryMatchFunc(key, value)
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

// Handler set handler.
func (m *matcher) Handler(fn func(w http.ResponseWriter, r *http.Request)) {
	m.handler = http.HandlerFunc(fn)
}

// Response set handler which return response (status and body).
func (m *matcher) Response(status int, body any) {
	var (
		b   []byte
		err error
	)
	switch v := body.(type) {
	case string:
		b = []byte(v)
	case []byte:
		b = v
	case nil:
		b = nil
	default:
		b, err = json.Marshal(v)
		if err != nil {
			m.router.t.Fatalf("failed to convert message: %v", err)
		}
	}
	fn := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write(b)
	}
	m.handler = http.HandlerFunc(fn)
}

// ResponseString set handler which return response (status and string-body).
func (m *matcher) ResponseString(status int, body string) {
	b := []byte(body)
	m.Response(status, b)
}

// ResponseStringf set handler which return response (status and sprintf-ed-body).
func (m *matcher) ResponseStringf(status int, format string, a ...any) {
	b := []byte(fmt.Sprintf(format, a...))
	m.Response(status, b)
}

type responseExampleConfig struct {
	status string
}

func newResponseExampleConfig() *responseExampleConfig {
	return &responseExampleConfig{status: "*"}
}

type responseExampleOption func(c *responseExampleConfig) error

// Status specify the example response to use by status code
func Status(pattern string) responseExampleOption {
	return func(c *responseExampleConfig) error {
		c.status = pattern
		return nil
	}
}

// ResponseExample set handler which return response using examples of OpenAPI v3 Document
func (m *matcher) ResponseExample(opts ...responseExampleOption) {
	if m.router.openapi3Doc == nil {
		m.router.t.Error("no OpenAPI v3 document is set")
		return
	}
	c := newResponseExampleConfig()
	for _, opt := range opts {
		if err := opt(c); err != nil {
			m.router.t.Error(err)
			return
		}
	}
	doc := *m.router.openapi3Doc
	v3, errs := doc.BuildV3Model()
	if errs != nil {
		m.router.t.Errorf("failed to build OpenAPI v3 model: %v", errors.Join(errs...))
		return
	}
	fn := func(w http.ResponseWriter, r *http.Request) {
		pathItem, errs, pathValue := paths.FindPath(r, &v3.Model)
		if pathItem == nil || errs != nil {
			var err error
			for _, e := range errs {
				err = errors.Join(err, e)
			}
			m.router.t.Errorf("failed to find route for %v %v: %v", r.Method, r.URL.Path, err)
			return
		}
		op, ok := pathItem.GetOperations().Get(strings.ToLower(r.Method))
		if !ok {
			m.router.t.Errorf("failed to find route (%v %v) operation of method: %s", r.Method, pathValue, r.Method)
			return
		}
		s, res := matchOne(op.Responses, c.status)
		if res == nil {
			m.router.t.Errorf("failed to find route (%v %v) response of status %s", r.Method, pathValue, c.status)
			return
		}
		status, err := strconv.Atoi(s)
		if err != nil {
			m.router.t.Error(err)
			return
		}

		mime := r.Header.Get("Content-Type")
		var e *base.Example
		if res.Content != nil {
			mt, ok := res.Content.Get(mime)
			if !ok {
				p := res.Content.First()
				mime, mt = p.Key(), p.Value()
			}
			if mt == nil {
				m.router.t.Errorf("failed to find route (%v %v %v) mimeType", status, r.Method, pathValue)
				return
			}
			if mt.Examples.Len() == 0 {
				m.router.t.Errorf("failed to find route (%v %v %v) example", status, r.Method, pathValue)
				return
			}
			e = one(mt.Examples)
		}
		var b []byte
		switch {
		case e == nil:
			b = nil
		case strings.Contains(mime, "text"):
			b = []byte(e.Value.Value)
		default:
			b, err = openapijson.YAMLNodeToJSON(e.Value, "  ")
			if err != nil {
				m.router.t.Errorf("failed to marshal body of route (%v %v %v)", status, r.Method, pathValue)
				return
			}
		}

		w.Header().Set("Content-Type", mime)
		w.WriteHeader(status)
		_, _ = w.Write(b)
	}
	m.handler = http.HandlerFunc(fn)
}

// ResponseExample set handler which return response using examples of OpenAPI v3 Document
func (rt *Router) ResponseExample(opts ...responseExampleOption) {
	m := &matcher{
		matchFuncs: []matchFunc{func(_ *http.Request) bool { return true }},
		router:     rt,
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.matchers = append(rt.matchers, m)
	m.ResponseExample(opts...)
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

// ClearRequests clear []*http.Request received by router.
func (rt *Router) ClearRequests() {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	rt.requests = nil
	for _, m := range rt.matchers {
		m.ClearRequests()
	}
}

// ClearRequests returns []*http.Request received by matcher.
func (m *matcher) ClearRequests() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	requests := []*http.Request{}
L:
	for _, r := range m.router.requests {
		for _, mr := range m.requests {
			if r == mr {
				continue L
			}
		}
		requests = append(requests, r)
	}
	m.router.requests = requests
	m.requests = nil
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
	return func(r *http.Request) bool {
		return wildcard.MatchSimple(path, r.URL.Path)
	}
}

func queryMatchFunc(key, value string) matchFunc {
	return func(r *http.Request) bool {
		return r.URL.Query().Get(key) == value
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

func one[K comparable, V *base.Example](m *orderedmap.Map[K, V]) V {
	l := m.Len()
	i := rand.Intn(l)
	for p := range orderedmap.Iterate(context.Background(), m) {
		if i == 0 {
			return p.Value()
		}
		i--
	}
	return nil
}

// matchOne returns match one randomly from map.
func matchOne(r *v3.Responses, pattern string) (string, *v3.Response) {
	m := r.Codes
	for p := range orderedmap.Iterate(context.Background(), m) {
		if wildcard.MatchSimple(pattern, p.Key()) {
			return p.Key(), p.Value()
		}
	}
	return "", nil
}

func withCloneReq(fn matchFunc) matchFunc {
	return func(r *http.Request) bool {
		r2 := cloneReq(r)
		return fn(r2)
	}
}
