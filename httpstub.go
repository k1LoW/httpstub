package httpstub

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	mrand "math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	wildcard "github.com/IGLOU-EU/go-wildcard/v2"
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
	openAPI3Doc                         libopenapi.Document
	openAPI3Validator                   validator.Validator
	skipValidateRequest                 bool
	skipValidateResponse                bool
	prependOnce                         bool
	addr                                string
	basePath                            string
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

	// Set skipCircularReferenceCheck first
	for _, opt := range opts {
		tmp := &config{}
		_ = opt(tmp)
		if tmp.skipCircularReferenceCheck {
			if err := opt(c); err != nil {
				t.Fatal(err)
			}
			break
		}
	}

	for _, opt := range opts {
		if err := opt(c); err != nil {
			t.Fatal(err)
		}
	}

	rt := &Router{
		t:                    t,
		useTLS:               c.useTLS,
		cacert:               c.cacert,
		cert:                 c.cert,
		key:                  c.key,
		clientCacert:         c.clientCacert,
		clientCert:           c.clientCert,
		clientKey:            c.clientKey,
		openAPI3Doc:          c.openAPI3Doc,
		openAPI3Validator:    c.openAPI3Validator,
		skipValidateRequest:  c.skipValidateRequest,
		skipValidateResponse: c.skipValidateResponse,
		addr:                 c.addr,
		basePath:             c.basePath,
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
		var h http.Handler = rt
		if rt.basePath != "" {
			mux := http.NewServeMux()
			mux.Handle(rt.basePath+"/", http.StripPrefix(rt.basePath, rt))
			h = mux
		}
		rt.server = httptest.NewUnstartedServer(h)
		if rt.addr != "" {
			if err := rt.server.Listener.Close(); err != nil {
				rt.t.Fatal(err)
				return nil
			}
			ln, err := net.Listen("tcp", rt.addr)
			if err != nil {
				rt.t.Fatal(err)
				return nil
			}
			rt.server.Listener = ln
		}

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
			transport, ok := client.Transport.(*http.Transport)
			if !ok {
				panic("failed to type assert to *http.Transport")
			}
			transport.TLSClientConfig.RootCAs = certpool
		}
		// client certificates
		if rt.clientCert != nil && rt.clientKey != nil {
			cert, err := tls.X509KeyPair(rt.clientCert, rt.clientKey)
			if err != nil {
				panic(err)
			}
			client := rt.server.Client()
			transport, ok := client.Transport.(*http.Transport)
			if !ok {
				panic("failed to type assert to *http.Transport")
			}
			transport.TLSClientConfig.Certificates = []tls.Certificate{cert}
		}
	} else {
		var h http.Handler = rt
		if rt.basePath != "" {
			mux := http.NewServeMux()
			mux.Handle(rt.basePath+"/", http.StripPrefix(rt.basePath, rt))
			h = mux
		}
		rt.server = httptest.NewUnstartedServer(h)
		if rt.addr != "" {
			if err := rt.server.Listener.Close(); err != nil {
				rt.t.Fatal(err)
				return nil
			}
			ln, err := net.Listen("tcp", rt.addr)
			if err != nil {
				rt.t.Fatal(err)
				return nil
			}
			rt.server.Listener = ln
		}
		rt.server.Start()
	}
	client := rt.server.Client()
	tp, ok := client.Transport.(*http.Transport)
	if !ok {
		panic("failed to type assert to *http.Transport")
	}
	client.Transport = newTransport(rt.server.URL, rt.basePath, tp)
	rt.URL = rt.server.URL
	return rt.server
}

// TLSServer returns TLS *httptest.Server with *Router set.
func (rt *Router) TLSServer() *httptest.Server {
	rt.useTLS = true
	return rt.Server()
}

// Close shuts down *httptest.Server.
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
	rt.addMatcher(m)
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
	rt.addMatcher(m)
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
		router:     rt,
	}
	rt.addMatcher(m)
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
	rt.addMatcher(m)
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

// Status specify the example response to use by status code.
func Status(pattern string) responseExampleOption {
	return func(c *responseExampleConfig) error {
		c.status = pattern
		return nil
	}
}

// ResponseExample set handler which return response using examples of OpenAPI v3 Document.
func (m *matcher) ResponseExample(opts ...responseExampleOption) {
	if m.router.openAPI3Doc == nil {
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
	doc := m.router.openAPI3Doc
	v3m, err := doc.BuildV3Model()
	if err != nil {
		m.router.t.Errorf("failed to build OpenAPI v3 model: %v", err)
		return
	}
	fn := func(w http.ResponseWriter, r *http.Request) {
		pathItem, errs, pathValue := paths.FindPath(r, &v3m.Model)
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
		status, e, contentType, err := matchOneExample(r, op.Responses, c.status)
		if err != nil {
			m.router.t.Errorf("failed to find route (%v %v) response: %w", r.Method, pathValue, err)
			return
		}
		var b []byte
		switch {
		case e == nil:
			b = nil
		case strings.Contains(contentType, "text"):
			b = []byte(e.Value.Value)
		default:
			b, err = openapijson.YAMLNodeToJSON(e.Value, "  ")
			if err != nil {
				m.router.t.Errorf("failed to marshal body of route (%v %v %v)", status, r.Method, pathValue)
				return
			}
		}

		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = w.Write(b)
	}
	m.handler = http.HandlerFunc(fn)
}

// ResponseExample set handler which return response using examples of OpenAPI v3 Document.
func (rt *Router) ResponseExample(opts ...responseExampleOption) {
	m := &matcher{
		matchFuncs: []matchFunc{func(_ *http.Request) bool { return true }},
		router:     rt,
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.addMatcher(m)
	m.ResponseExample(opts...)
}

// ResponseDynamic set handler which return response using schema of OpenAPI v3 Document.
func (m *matcher) ResponseDynamic(opts ...responseExampleOption) {
	if m.router.openAPI3Doc == nil {
		m.router.t.Fatal("no OpenAPI v3 document is set")
		return
	}
	c := newResponseExampleConfig()
	for _, opt := range opts {
		if err := opt(c); err != nil {
			m.router.t.Error(err)
			return
		}
	}
	doc := m.router.openAPI3Doc
	v3m, err := doc.BuildV3Model()
	if err != nil {
		m.router.t.Fatalf("failed to build OpenAPI v3 model: %v", err)
		return
	}
	fn := func(w http.ResponseWriter, r *http.Request) {
		pathItem, errs, pathValue := paths.FindPath(r, &v3m.Model)
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
		status, res, contentType, err := matchOne(r, op.Responses, c.status)
		if res == nil {
			m.router.t.Errorf("failed to find route (%v %v) response: %w", r.Method, pathValue, err)
			return
		}
		var schemaProxy *base.SchemaProxy
		if res.Content != nil {
			mt, ok := res.Content.Get(contentType)
			if !ok {
				p := res.Content.First()
				contentType, mt = p.Key(), p.Value()
			}
			if mt == nil {
				m.router.t.Errorf("failed to find route (%v %v %v) mimeType", status, r.Method, pathValue)
				return
			}
			schemaProxy = mt.Schema
		}

		var b []byte
		if schemaProxy == nil {
			b = nil
		} else {
			schema := schemaProxy.Schema()
			data, err := generateFromSchema(schema, 0)
			if err != nil {
				m.router.t.Errorf("failed to generate data from schema: %v", err)
				return
			}

			switch {
			case data == nil:
				b = nil
			case strings.Contains(contentType, "text"):
				b = fmt.Appendf(b, "%v", data)
			default:
				b, err = json.Marshal(data)
				if err != nil {
					m.router.t.Errorf("failed to marshal body of route (%v %v %v)", status, r.Method, pathValue)
					return
				}
			}
		}

		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = w.Write(b)
	}
	m.handler = http.HandlerFunc(fn)
}

// ResponseDynamic set handler which return response using schema of OpenAPI v3 Document.
func (rt *Router) ResponseDynamic(opts ...responseExampleOption) {
	m := &matcher{
		matchFuncs: []matchFunc{func(_ *http.Request) bool { return true }},
		router:     rt,
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.addMatcher(m)
	m.ResponseDynamic(opts...)
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

// Prepend prepend matcher.
func (rt *Router) Prepend() *Router {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.prependOnce = true
	return rt
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

func (rt *Router) addMatcher(m *matcher) {
	if rt.prependOnce {
		rt.matchers = append([]*matcher{m}, rt.matchers...)
		rt.prependOnce = false
		return
	}
	rt.matchers = append(rt.matchers, m)
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
		return wildcard.Match(path, r.URL.Path)
	}
}

func queryMatchFunc(key, value string) matchFunc {
	return func(r *http.Request) bool {
		return r.URL.Query().Get(key) == value
	}
}

type transport struct {
	URL      *url.URL
	basePath string
	tp       *http.Transport
}

func newTransport(rawURL string, basePath string, tp *http.Transport) http.RoundTripper {
	u, _ := url.Parse(rawURL)
	return &transport{
		URL:      u,
		basePath: basePath,
		tp:       tp,
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
	n, err := rand.Int(rand.Reader, big.NewInt(int64(l)))
	if err != nil {
		// Return the first element if an error occurs
		for p := range orderedmap.Iterate(context.Background(), m) {
			return p.Value()
		}
		return nil
	}
	i := int(n.Int64())
	for p := range orderedmap.Iterate(context.Background(), m) {
		if i == 0 {
			return p.Value()
		}
		i--
	}
	return nil
}

// matchOne returns match one randomly from map.
func matchOne(req *http.Request, r *v3.Responses, pattern string) (status int, res *v3.Response, contentType string, err error) {
	m := r.Codes
	var matched []orderedmap.Pair[string, *v3.Response]
	for p := range orderedmap.Iterate(context.Background(), m) {
		if wildcard.Match(pattern, p.Key()) {
			matched = append(matched, p)
		}
	}
	if len(matched) == 0 {
		return 0, nil, "", fmt.Errorf("failed to find response matching pattern: %s", pattern)
	}
	idx := mrand.Intn(len(matched)) //nolint:gosec
	status, err = strconv.Atoi(matched[idx].Key())
	if err != nil {
		return 0, nil, "", fmt.Errorf("invalid status code: %w", err)
	}
	res = matched[idx].Value()
	accepts := strings.Split(req.Header.Get("Accept"), ",")
	var contentTypes []string
	for _, a := range accepts {
		contentTypes = append(contentTypes, strings.TrimSpace(a))
	}
	return status, res, contentType, nil
}

// matchOneExample returns match one randomly from map.
func matchOneExample(req *http.Request, r *v3.Responses, pattern string) (status int, example *base.Example, contentType string, err error) {
	m := r.Codes
	var matched []orderedmap.Pair[string, *v3.Response]
	for p := range orderedmap.Iterate(context.Background(), m) {
		if wildcard.Match(pattern, p.Key()) {
			matched = append(matched, p)
		}
	}
	if len(matched) == 0 {
		return 0, nil, "", fmt.Errorf("failed to find response matching pattern: %s", pattern)
	}
	idx := mrand.Intn(len(matched)) //nolint:gosec
	status, err = strconv.Atoi(matched[idx].Key())
	if err != nil {
		return 0, nil, "", fmt.Errorf("invalid status code: %w", err)
	}
	res := matched[idx].Value()
	accepts := strings.Split(req.Header.Get("Accept"), ",")
	var contentTypes []string
	for _, a := range accepts {
		contentTypes = append(contentTypes, strings.TrimSpace(a))
	}
	var e *base.Example
	if res.Content != nil {
		var (
			mt *v3.MediaType
			ok bool
		)
		for _, ct := range contentTypes {
			mt, ok = res.Content.Get(ct)
			if !ok {
				continue
			}
			contentType = ct
			if mt.Examples.Len() == 0 {
				continue
			}
			e = one(mt.Examples)
			break
		}
		if mt == nil {
			for p := range orderedmap.Iterate(context.Background(), res.Content) {
				mt = p.Value()
				contentType = p.Key()
				if mt.Examples.Len() == 0 {
					continue
				}
				e = one(mt.Examples)
				break
			}
		}
		if mt == nil {
			return 0, nil, "", fmt.Errorf("failed to find example")
		}
	}

	return status, e, contentType, nil
}

func withCloneReq(fn matchFunc) matchFunc {
	return func(r *http.Request) bool {
		r2 := cloneReq(r)
		return fn(r2)
	}
}

// Constants for random data generation
const (
	maxDepth              = 10
	defaultStringLength   = 10
	defaultArrayMinLength = 0
	defaultArrayMaxLength = 3
	defaultNumberMin      = 0.0
	defaultNumberMax      = 100.0
	defaultIntegerMin     = 0
	defaultIntegerMax     = 100
)

// randomInt generates a random integer between min and max (inclusive)
func randomInt(min, max int64) int64 {
	if min > max {
		min, max = max, min
	}
	if min == max {
		return min
	}
	n, err := rand.Int(rand.Reader, big.NewInt(max-min+1))
	if err != nil {
		return min
	}
	return n.Int64() + min
}

// randomFloat generates a random float64 between min and max
func randomFloat(min, max float64) float64 {
	if min > max {
		min, max = max, min
	}
	if min == max {
		return min
	}
	// Generate random bytes and convert to float in [0, 1)
	var b [8]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return min
	}
	// Convert bytes to uint64
	var n uint64
	for i := range 8 {
		n = (n << 8) | uint64(b[i])
	}
	// Convert to float in [0, 1)
	f := float64(n) / float64(^uint64(0))
	return min + f*(max-min)
}

// randomString generates a random alphanumeric string of the specified length
func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		n := randomInt(0, int64(len(charset)-1))
		b[i] = charset[n]
	}
	return string(b)
}

// randomBool generates a random boolean value
func randomBool() bool {
	return randomInt(0, 1) == 1
}

// generateDate generates a random date in YYYY-MM-DD format
func generateDate() string {
	year := randomInt(2020, 2030)
	month := randomInt(1, 12)
	day := randomInt(1, 28) // Use 28 to avoid invalid dates
	return fmt.Sprintf("%04d-%02d-%02d", year, month, day)
}

// generateDateTime generates a random datetime in RFC3339 format
func generateDateTime() string {
	year := randomInt(2020, 2030)
	month := randomInt(1, 12)
	day := randomInt(1, 28)
	hour := randomInt(0, 23)
	minute := randomInt(0, 59)
	second := randomInt(0, 59)
	return fmt.Sprintf("%04d-%02d-%02dT%02d:%02d:%02dZ", year, month, day, hour, minute, second)
}

// generateEmail generates a random email address
func generateEmail() string {
	return fmt.Sprintf("user%s@example.com", randomString(8))
}

// generateUUID generates a random UUID v4
func generateUUID() string {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return "00000000-0000-0000-0000-000000000000"
	}
	// Set version (4) and variant bits
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// generateFromSchema generates random data based on OpenAPI Schema
func generateFromSchema(schema *base.Schema, depth int) (any, error) {
	if schema == nil {
		return nil, nil
	}

	// Check recursion depth
	if depth > maxDepth {
		return nil, nil
	}

	// Handle nullable
	if schema.Nullable != nil && *schema.Nullable {
		if randomBool() {
			return nil, nil
		}
	}

	// Handle type array (OpenAPI 3.1.x)
	if len(schema.Type) > 0 {
		// Check if "null" is in the type array
		hasNull := false
		var primaryType string
		for _, t := range schema.Type {
			if t == "null" {
				hasNull = true
			} else {
				primaryType = t
			}
		}
		if hasNull && randomBool() {
			return nil, nil
		}
		// Use the primary type
		if primaryType != "" {
			return generateByType(schema, primaryType, depth)
		}
	}

	// Handle enum
	if len(schema.Enum) > 0 {
		idx := randomInt(0, int64(len(schema.Enum)-1))
		return schema.Enum[idx].Value, nil
	}

	// Handle allOf/anyOf/oneOf (use first element only)
	if len(schema.AllOf) > 0 && schema.AllOf[0].Schema() != nil {
		return generateFromSchema(schema.AllOf[0].Schema(), depth+1)
	}
	if len(schema.AnyOf) > 0 && schema.AnyOf[0].Schema() != nil {
		return generateFromSchema(schema.AnyOf[0].Schema(), depth+1)
	}
	if len(schema.OneOf) > 0 && schema.OneOf[0].Schema() != nil {
		return generateFromSchema(schema.OneOf[0].Schema(), depth+1)
	}

	// Determine type from schema
	if len(schema.Type) > 0 {
		return generateByType(schema, schema.Type[0], depth)
	}

	// If no type specified, try to infer from properties
	if schema.Properties != nil && schema.Properties.Len() > 0 {
		return generateObject(schema, depth)
	}
	if schema.Items != nil && schema.Items.IsA() {
		return generateArray(schema, depth)
	}

	// Default to empty object
	return map[string]any{}, nil
}

// generateByType generates data based on the specified type
func generateByType(schema *base.Schema, typeName string, depth int) (any, error) {
	switch typeName {
	case "string":
		return generateString(schema), nil
	case "number":
		return generateNumber(schema), nil
	case "integer":
		return generateInteger(schema), nil
	case "boolean":
		return generateBoolean(), nil
	case "array":
		return generateArray(schema, depth)
	case "object":
		return generateObject(schema, depth)
	default:
		return nil, fmt.Errorf("unsupported type: %s", typeName)
	}
}

// generateString generates a random string based on schema constraints
func generateString(schema *base.Schema) string {
	// Handle enum
	if len(schema.Enum) > 0 {
		idx := randomInt(0, int64(len(schema.Enum)-1))
		return fmt.Sprintf("%v", schema.Enum[idx].Value)
	}

	// Handle format
	if schema.Format != "" {
		switch schema.Format {
		case "date":
			return generateDate()
		case "date-time":
			return generateDateTime()
		case "email":
			return generateEmail()
		case "uuid":
			return generateUUID()
		}
	}

	// Determine length
	length := defaultStringLength
	if schema.MinLength != nil && *schema.MinLength > 0 {
		length = int(*schema.MinLength)
	}
	if schema.MaxLength != nil && *schema.MaxLength > 0 {
		maxLen := int(*schema.MaxLength)
		if schema.MinLength != nil {
			length = int(randomInt(int64(*schema.MinLength), int64(maxLen)))
		} else {
			length = int(randomInt(0, int64(maxLen)))
		}
	}

	if length < 0 {
		length = defaultStringLength
	}

	return randomString(length)
}

// generateNumber generates a random float64 based on schema constraints
func generateNumber(schema *base.Schema) float64 {
	// Handle enum
	if len(schema.Enum) > 0 {
		idx := randomInt(0, int64(len(schema.Enum)-1))
		// Try to parse as float
		if f, err := strconv.ParseFloat(schema.Enum[idx].Value, 64); err == nil {
			return f
		}
		return defaultNumberMin
	}

	min := defaultNumberMin
	max := defaultNumberMax

	if schema.Minimum != nil {
		min = *schema.Minimum
		if schema.ExclusiveMinimum != nil {
			if schema.ExclusiveMinimum.IsA() && schema.ExclusiveMinimum.A {
				min += 0.01
			}
		}
	}
	if schema.Maximum != nil {
		max = *schema.Maximum
		if schema.ExclusiveMaximum != nil {
			if schema.ExclusiveMaximum.IsA() && schema.ExclusiveMaximum.A {
				max -= 0.01
			}
		}
	}

	value := randomFloat(min, max)

	// Handle multipleOf
	if schema.MultipleOf != nil && *schema.MultipleOf > 0 {
		value = float64(int64(value/(*schema.MultipleOf))) * (*schema.MultipleOf)
	}

	return value
}

// generateInteger generates a random int64 based on schema constraints
func generateInteger(schema *base.Schema) int64 {
	// Handle enum
	if len(schema.Enum) > 0 {
		idx := randomInt(0, int64(len(schema.Enum)-1))
		// Try to parse as int
		if i, err := strconv.ParseInt(schema.Enum[idx].Value, 10, 64); err == nil {
			return i
		}
		return defaultIntegerMin
	}

	min := int64(defaultIntegerMin)
	max := int64(defaultIntegerMax)

	if schema.Minimum != nil {
		min = int64(*schema.Minimum)
		if schema.ExclusiveMinimum != nil {
			if schema.ExclusiveMinimum.IsA() && schema.ExclusiveMinimum.A {
				min++
			}
		}
	}
	if schema.Maximum != nil {
		max = int64(*schema.Maximum)
		if schema.ExclusiveMaximum != nil {
			if schema.ExclusiveMaximum.IsA() && schema.ExclusiveMaximum.A {
				max--
			}
		}
	}

	value := randomInt(min, max)

	// Handle multipleOf
	if schema.MultipleOf != nil && *schema.MultipleOf > 0 {
		multipleOf := int64(*schema.MultipleOf)
		if multipleOf > 0 {
			value = (value / multipleOf) * multipleOf
		}
	}

	return value
}

// generateBoolean generates a random boolean value
func generateBoolean() bool {
	return randomBool()
}

// generateArray generates a random array based on schema constraints
func generateArray(schema *base.Schema, depth int) ([]any, error) {
	if depth > maxDepth {
		return []any{}, nil
	}

	// Determine array length
	minItems := defaultArrayMinLength
	maxItems := defaultArrayMaxLength

	if schema.MinItems != nil && *schema.MinItems >= 0 {
		minItems = int(*schema.MinItems)
	}
	if schema.MaxItems != nil && *schema.MaxItems >= 0 {
		maxItems = int(*schema.MaxItems)
		if maxItems < minItems {
			maxItems = minItems
		}
	}

	length := int(randomInt(int64(minItems), int64(maxItems)))

	result := make([]any, length)

	// Generate items
	if schema.Items != nil && schema.Items.IsA() {
		itemSchema := schema.Items.A.Schema()
		for i := range length {
			item, err := generateFromSchema(itemSchema, depth+1)
			if err != nil {
				return nil, err
			}
			result[i] = item
		}
	}

	return result, nil
}

// generateObject generates a random object based on schema constraints
func generateObject(schema *base.Schema, depth int) (map[string]any, error) {
	if depth > maxDepth {
		return map[string]any{}, nil
	}

	result := make(map[string]any)

	if schema.Properties == nil {
		return result, nil
	}

	// Collect required properties
	requiredProps := make(map[string]bool)
	for _, req := range schema.Required {
		requiredProps[req] = true
	}

	// Generate properties
	for pair := range orderedmap.Iterate(context.Background(), schema.Properties) {
		propName := pair.Key()
		propSchemaProxy := pair.Value()

		// Include required properties or 50% chance for optional properties
		if requiredProps[propName] || randomBool() {
			if propSchemaProxy != nil {
				propSchema := propSchemaProxy.Schema()
				value, err := generateFromSchema(propSchema, depth+1)
				if err != nil {
					return nil, err
				}
				result[propName] = value
			}
		}
	}

	return result, nil
}
