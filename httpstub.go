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
	"time"

	wildcard "github.com/IGLOU-EU/go-wildcard/v2"
	"github.com/pb33f/libopenapi"
	validator "github.com/pb33f/libopenapi-validator"
	"github.com/pb33f/libopenapi-validator/paths"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	openapijson "github.com/pb33f/libopenapi/json"
	"github.com/pb33f/libopenapi/orderedmap"
	"github.com/pb33f/libopenapi/renderer"
	"go.yaml.in/yaml/v4"
)

var (
	_ http.Handler = (*Router)(nil)
	_ TB           = (testing.TB)(nil)
)

// ResponseMode defines how to generate responses from OpenAPI documents.
type ResponseMode int

const (
	// AlwaysGenerate always generates responses from schemas.
	// Examples are ignored.
	// This is the default behavior.
	AlwaysGenerate ResponseMode = iota
	// ExamplesOnly uses only explicit examples from the OpenAPI document.
	// If no example is found, an error is returned.
	ExamplesOnly
	// PreferExamples prefers examples but falls back to schema generation.
	PreferExamples
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
	mockGenerator                       *renderer.MockGenerator
	rng                                 *mrand.Rand
	responseMode                        ResponseMode
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

	mode := c.responseMode
	if mode == 0 {
		mode = AlwaysGenerate
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
		responseMode:         mode,
	}
	if err := rt.setOpenApi3Vaildator(); err != nil {
		t.Fatal(err)
	}

	// Initialize MockGenerator (use JSON mock type) and seed math/rand for deterministic example selection in tests
	mg := renderer.NewMockGenerator(renderer.JSON)
	var seed int64
	if c.seed != 0 {
		seed = c.seed
	} else {
		seed = time.Now().UnixNano()
	}
	mg.SetSeed(seed)
	rt.mockGenerator = mg
	rt.rng = mrand.New(mrand.NewSource(seed))

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
	return &responseExampleConfig{
		status: "*",
	}
}

type responseExampleOption func(c *responseExampleConfig) error

// Status specify the example response to use by status code.
func Status(pattern string) responseExampleOption {
	return func(c *responseExampleConfig) error {
		c.status = pattern
		return nil
	}
}

// selectMatchedResponses returns responses that match the given status pattern.
func (m *matcher) selectMatchedResponses(responses *v3.Responses, pattern string) ([]orderedmap.Pair[string, *v3.Response], error) {
	resMap := responses.Codes
	var matchedResps []orderedmap.Pair[string, *v3.Response]
	for p := range orderedmap.Iterate(context.Background(), resMap) {
		if wildcard.Match(pattern, p.Key()) {
			matchedResps = append(matchedResps, p)
		}
	}
	if len(matchedResps) == 0 {
		return nil, fmt.Errorf("failed to find response matching pattern: %s", pattern)
	}
	return matchedResps, nil
}

// pickStatusAndResponse selects one of the matched responses (randomly) and returns its status and response.
func (m *matcher) pickStatusAndResponse(matchedResps []orderedmap.Pair[string, *v3.Response]) (int, *v3.Response, string, error) {
	idx := m.router.rng.Intn(len(matchedResps)) //nolint:gosec
	statusStr := matchedResps[idx].Key()
	status, err := strconv.Atoi(statusStr)
	if err != nil {
		return 0, nil, "", fmt.Errorf("invalid status code: %w", err)
	}
	return status, matchedResps[idx].Value(), "", nil
}

// pickMediaType determines the media type to use from response content and request Accept header.
func (m *matcher) pickMediaType(res *v3.Response, req *http.Request) (*v3.MediaType, string) {
	if res.Content == nil {
		return nil, ""
	}
	accepts := strings.Split(req.Header.Get("Accept"), ",")
	var contentTypes []string
	for _, a := range accepts {
		contentTypes = append(contentTypes, strings.TrimSpace(a))
	}

	var mt *v3.MediaType
	var contentType string
	for _, ct := range contentTypes {
		if tmp, ok := res.Content.Get(ct); ok {
			mt = tmp
			contentType = ct
			break
		}
	}
	if mt == nil {
		if p := res.Content.Oldest(); p != nil {
			mt = p.Value
			contentType = p.Key
		}
	}
	return mt, contentType
}

// genMockFromMediaType generates a mock yaml.Node from a media type's schema.
func (m *matcher) genMockFromMediaType(mt *v3.MediaType) (*yaml.Node, error) {
	if mt == nil || mt.Schema == nil {
		return nil, fmt.Errorf("no schema available to generate mock")
	}
	mockBytes, genErr := m.router.mockGenerator.GenerateMock(mt.Schema.Schema(), "")
	if genErr != nil {
		return nil, fmt.Errorf("failed to generate mock data: %w", genErr)
	}
	var mockNode yaml.Node
	if err := yaml.Unmarshal(mockBytes, &mockNode); err != nil {
		return nil, fmt.Errorf("failed to unmarshal generated mock: %w", err)
	}
	return &mockNode, nil
}

// selectExample selects the first available example from the media type in deterministic order.
// Returns true if an example was successfully selected, false otherwise.
func (m *matcher) selectExample(mt *v3.MediaType, status int, contentType string) (int, *yaml.Node, string, bool) {
	if mt == nil {
		return 0, nil, "", false
	}
	if mt.Examples != nil && mt.Examples.Len() > 0 {
		for p := range orderedmap.Iterate(context.Background(), mt.Examples) {
			ex := p.Value()
			if ex != nil && ex.Value != nil {
				return status, ex.Value, contentType, true
			}
		}
	}
	if mt.Example != nil {
		return status, mt.Example, contentType, true
	}
	return 0, nil, "", false
}

// generateFromSchema generates a mock response from the schema of the media type.
func (m *matcher) generateFromSchema(mt *v3.MediaType, status int, contentType string) (int, *yaml.Node, string, error) {
	if mt != nil && mt.Schema != nil {
		if node, e := m.genMockFromMediaType(mt); e == nil {
			return status, node, contentType, nil
		} else {
			return 0, nil, "", e
		}
	}
	return 0, nil, "", fmt.Errorf("no schema available to generate mock for response status %d", status)
}

// prepareResponse is a helper method to prepare response components (status, media type, content type).
func (m *matcher) prepareResponse(req *http.Request, responses *v3.Responses, pattern string) (status int, mt *v3.MediaType, contentType string, err error) {
	matchedResps, err := m.selectMatchedResponses(responses, pattern)
	if err != nil {
		return 0, nil, "", err
	}

	status, res, _, err := m.pickStatusAndResponse(matchedResps)
	if err != nil {
		return 0, nil, "", err
	}

	mt, contentType = m.pickMediaType(res, req)
	return status, mt, contentType, nil
}

// findResponseContentDynamic is a helper method to generate random data from schema only (no examples).
func (m *matcher) findResponseContentDynamic(req *http.Request, responses *v3.Responses, pattern string) (status int, exampleNode *yaml.Node, contentType string, err error) {
	status, mt, contentType, err := m.prepareResponse(req, responses, pattern)
	if err != nil {
		return 0, nil, "", err
	}

	// Always generate from schema (no examples)
	return m.generateFromSchema(mt, status, contentType)
}

// findResponseExample is a helper method to find the example only (no fallback to generation).
func (m *matcher) findResponseExample(req *http.Request, responses *v3.Responses, pattern string) (status int, exampleNode *yaml.Node, contentType string, err error) {
	status, mt, contentType, err := m.prepareResponse(req, responses, pattern)
	if err != nil {
		return 0, nil, "", err
	}

	// Only use examples (deterministic selection)
	if s, node, ct, ok := m.selectExample(mt, status, contentType); ok {
		return s, node, ct, nil
	}

	return 0, nil, "", fmt.Errorf("no example found for response status %d", status)
}

// findResponseContentAuto is a helper method to find the example or generate random data.
// It prefers examples, but falls back to schema-based generation if no example is found.
func (m *matcher) findResponseContentAuto(req *http.Request, responses *v3.Responses, pattern string) (status int, exampleNode *yaml.Node, contentType string, err error) {
	status, mt, contentType, err := m.prepareResponse(req, responses, pattern)
	if err != nil {
		return 0, nil, "", err
	}

	// Prefer examples if present (deterministic selection)
	if s, node, ct, ok := m.selectExample(mt, status, contentType); ok {
		return s, node, ct, nil
	}

	// Fallback to generation from schema
	return m.generateFromSchema(mt, status, contentType)
}

// ResponseDynamic set handler which return response from OpenAPI v3 Document.
// The response mode is determined by the Router's responseMode.
func (m *matcher) ResponseDynamic(opts ...responseExampleOption) {
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
	regexCache := &sync.Map{}
	fn := func(w http.ResponseWriter, r *http.Request) {
		pathItem, errs, pathValue := paths.FindPath(r, &v3m.Model, regexCache)
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

		var status int
		var exampleNode *yaml.Node
		var contentType string
		var err error

		// Select response generation method based on router's response mode
		switch m.router.responseMode {
		case AlwaysGenerate:
			status, exampleNode, contentType, err = m.findResponseContentDynamic(r, op.Responses, c.status)
		case ExamplesOnly:
			status, exampleNode, contentType, err = m.findResponseExample(r, op.Responses, c.status)
		case PreferExamples:
			status, exampleNode, contentType, err = m.findResponseContentAuto(r, op.Responses, c.status)
		default:
			// This should never happen as responseMode is always set to a valid value in NewRouter
			m.router.t.Fatalf("invalid response mode: %v", m.router.responseMode)
			return
		}

		if err != nil {
			m.router.t.Errorf("failed to generate response for route (%v %v): %v", r.Method, pathValue, err)
			return
		}
		var b []byte
		if exampleNode != nil {
			b, err = openapijson.YAMLNodeToJSON(exampleNode, "  ")
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

// ResponseDynamic set handler which return response from OpenAPI v3 Document.
// The response mode is determined by the Router's responseMode.
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

func withCloneReq(fn matchFunc) matchFunc {
	return func(r *http.Request) bool {
		r2 := cloneReq(r)
		return fn(r2)
	}
}
