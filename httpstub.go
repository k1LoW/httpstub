package httpstub

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

var _ http.Handler = (*router)(nil)

type router struct {
	matchers    []*matcher
	server      *httptest.Server
	middlewares middlewareFuncs
	requests    []*http.Request
	t           *testing.T
}

type matcher struct {
	matchFuncs  []matchFunc
	handler     http.HandlerFunc
	middlewares middlewareFuncs
	requests    []*http.Request
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

func (rt *router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rt.t.Helper()
	r2 := cloneReq(r)
	rt.requests = append(rt.requests, r2)

	for _, m := range rt.matchers {
		match := true
		for _, fn := range m.matchFuncs {
			if !fn(r) {
				match = false
			}
		}
		if match {
			m.requests = append(m.requests, r2)
			mws := append(rt.middlewares, m.middlewares...)
			mws.then(m.handler).ServeHTTP(w, r)
			return
		}
	}
	dump, _ := httputil.DumpRequest(r, true)
	rt.t.Errorf("httpstub error: request did not match\n---REQUEST START---\n%s\n---REQUEST END---\n", string(dump))
}

func NewRouter(t *testing.T) *router {
	t.Helper()
	return &router{t: t}
}

func NewServer(t *testing.T) *router {
	t.Helper()
	rt := &router{t: t}
	_ = rt.Server()
	return rt
}

func (rt *router) Client() *http.Client {
	if rt.server == nil {
		rt.t.Error("server is not started yet")
		return nil
	}
	return rt.server.Client()
}

func (rt *router) Server() *httptest.Server {
	if rt.server == nil {
		rt.server = httptest.NewServer(rt)
	}
	client := rt.server.Client()
	client.Transport = newTransport(rt.server.URL)
	return rt.server
}

func (rt *router) Close() {
	if rt.server == nil {
		rt.t.Error("server is not started yet")
		return
	}
	rt.server.Close()
}

func (rt *router) Match(fn func(r *http.Request) bool) *matcher {
	m := &matcher{
		matchFuncs: []matchFunc{fn},
	}
	rt.matchers = append(rt.matchers, m)
	return m
}

func (m *matcher) Match(fn func(r *http.Request) bool) *matcher {
	m.matchFuncs = append(m.matchFuncs, fn)
	return m
}

func (rt *router) Method(method string) *matcher {
	fn := methodMatchFunc(method)
	m := &matcher{
		matchFuncs: []matchFunc{fn},
	}
	rt.matchers = append(rt.matchers, m)
	return m
}

func (m *matcher) Method(method string) *matcher {
	fn := methodMatchFunc(method)
	m.matchFuncs = append(m.matchFuncs, fn)
	return m
}

func (rt *router) Path(path string) *matcher {
	fn := pathMatchFunc(path)
	m := &matcher{
		matchFuncs: []matchFunc{fn},
	}
	rt.matchers = append(rt.matchers, m)
	return m
}

func (m *matcher) Path(path string) *matcher {
	fn := pathMatchFunc(path)
	m.matchFuncs = append(m.matchFuncs, fn)
	return m
}

func (rt *router) DefaultMiddleware(mw func(next http.HandlerFunc) http.HandlerFunc) {
	rt.middlewares = append(rt.middlewares, mw)
}

func (rt *router) DefaultHeader(key, value string) {
	mw := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add(key, value)
			next.ServeHTTP(w, r)
		}
	}
	rt.middlewares = append(rt.middlewares, mw)
}

func (m *matcher) Middleware(mw func(next http.HandlerFunc) http.HandlerFunc) *matcher {
	m.middlewares = append(m.middlewares, mw)
	return m
}

func (m *matcher) Header(key, value string) *matcher {
	mw := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add(key, value)
			next.ServeHTTP(w, r)
		}
	}
	m.middlewares = append(m.middlewares, mw)
	return m
}

func (m *matcher) Handler(fn func(w http.ResponseWriter, r *http.Request)) {
	m.handler = http.HandlerFunc(fn)
}

func (m *matcher) Response(status int, body []byte) {
	fn := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}
	m.handler = http.HandlerFunc(fn)
}

func (m *matcher) ResponseString(status int, body string) {
	b := []byte(body)
	m.Response(status, b)
}

func (rt *router) Requests() []*http.Request {
	return rt.requests
}

func (m *matcher) Requests() []*http.Request {
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
}

func newTransport(rawURL string) http.RoundTripper {
	u, _ := url.Parse(rawURL)
	return &transport{
		URL: u,
	}
}

func (t *transport) transport() http.RoundTripper {
	return http.DefaultTransport
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
