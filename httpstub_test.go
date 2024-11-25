package httpstub

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	mock_httpstub "github.com/k1LoW/httpstub/mock"
)

func TestStub(t *testing.T) {
	rt := NewRouter(t)
	rt.Method(http.MethodGet).Path("/api/v1/users/1").Header("Content-Type", "application/json").ResponseString(http.StatusOK, `{"name":"alice"}`)
	ts := rt.Server()
	t.Cleanup(func() {
		ts.Close()
	})
	tc := ts.Client()

	res, err := tc.Get("https://example.com/api/v1/users/1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		res.Body.Close()
	})
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}

	{
		got := res.StatusCode
		want := http.StatusOK
		if got != want {
			t.Errorf("got %v\nwant %v", got, want)
		}
	}
	{
		got := res.Header.Get("Content-Type")
		want := "application/json"
		if got != want {
			t.Errorf("got %v\nwant %v", got, want)
		}
	}
	{
		got := string(body)
		want := `{"name":"alice"}`
		if got != want {
			t.Errorf("got %v\nwant %v", got, want)
		}
	}
}

func TestResponse(t *testing.T) {
	tests := []struct {
		body any
	}{
		{`{"name":"alice"}`},
		{[]byte(`{"name":"alice"}`)},
		{map[string]string{"name": "alice"}},
		{struct {
			Name string `json:"name"`
		}{"alice"}},
	}
	for _, tt := range tests {
		rt := NewRouter(t)
		rt.Method(http.MethodGet).Path("/api/v1/users/1").Header("Content-Type", "application/json").Response(http.StatusOK, tt.body)
		ts := rt.Server()
		t.Cleanup(func() {
			ts.Close()
		})
		tc := ts.Client()

		res, err := tc.Get("https://example.com/api/v1/users/1")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			res.Body.Close()
		})
		body, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		{
			got := string(body)
			want := `{"name":"alice"}`
			if got != want {
				t.Errorf("got %v\nwant %v", got, want)
			}
		}
	}

}

func TestRouterMatch(t *testing.T) {
	rt := NewRouter(t)
	rt.Match(func(r *http.Request) bool {
		return r.Method == http.MethodGet
	}).Response(http.StatusAccepted, []byte(`{"message":"accepted"}`))
	ts := rt.Server()
	t.Cleanup(func() {
		ts.Close()
	})
	tc := ts.Client()

	res, err := tc.Get("https://example.com/api/v1/users/1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		res.Body.Close()
	})

	got := res.StatusCode
	want := http.StatusAccepted
	if got != want {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestMatcherMatch(t *testing.T) {
	rt := NewRouter(t)
	rt.Path("/api/v1/users/1").Match(func(r *http.Request) bool {
		return r.Method == http.MethodGet
	}).ResponseString(http.StatusAccepted, `{"message":"accepted"}`)
	ts := rt.Server()
	t.Cleanup(func() {
		ts.Close()
	})
	tc := ts.Client()

	res, err := tc.Get("https://example.com/api/v1/users/1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		res.Body.Close()
	})

	got := res.StatusCode
	want := http.StatusAccepted
	if got != want {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestMatcherMatchConsumingBody(t *testing.T) {
	t.Run("only Match", func(t *testing.T) {
		rt := NewRouter(t)
		rt.Match(func(r *http.Request) bool {
			b, err := io.ReadAll(r.Body)
			if err != nil {
				return false
			}
			return strings.Contains(string(b), "add")
		}).Response(http.StatusAccepted, nil)
		rt.Match(func(r *http.Request) bool {
			b, err := io.ReadAll(r.Body)
			if err != nil {
				return false
			}
			return strings.Contains(string(b), "subtract")
		}).Response(http.StatusAccepted, nil)
		ts := rt.Server()
		t.Cleanup(func() {
			ts.Close()
		})
		tc := ts.Client()
		res, err := tc.Post("https://example.com/jrpc", "application/json", strings.NewReader(`{"jsonrpc":"2.0", "method": "subtract", "params": {"a": 10, "b": 3}, "id": 123}`))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			res.Body.Close()
		})

		requests := rt.Requests()
		if len(requests) != 1 {
			t.Errorf("got %v\nwant %v", len(requests), 1)
		}
		b, err := io.ReadAll(requests[0].Body)
		if err != nil {
			t.Fatal(err)
		}
		got := string(b)
		want := `{"jsonrpc":"2.0", "method": "subtract", "params": {"a": 10, "b": 3}, "id": 123}`
		if got != want {
			t.Errorf("got %v\nwant %v", got, want)
		}
	})
	t.Run("Path and Match", func(t *testing.T) {
		rt := NewRouter(t)
		rt.Path("/jrpc").Match(func(r *http.Request) bool {
			b, err := io.ReadAll(r.Body)
			if err != nil {
				return false
			}
			return strings.Contains(string(b), "add")
		}).Response(http.StatusAccepted, nil)
		rt.Path("/jrpc").Match(func(r *http.Request) bool {
			b, err := io.ReadAll(r.Body)
			if err != nil {
				return false
			}
			return strings.Contains(string(b), "subtract")
		}).Response(http.StatusAccepted, nil)
		ts := rt.Server()
		t.Cleanup(func() {
			ts.Close()
		})
		tc := ts.Client()
		res, err := tc.Post("https://example.com/jrpc", "application/json", strings.NewReader(`{"jsonrpc":"2.0", "method": "subtract", "params": {"a": 10, "b": 3}, "id": 123}`))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			res.Body.Close()
		})

		requests := rt.Requests()
		if len(requests) != 1 {
			t.Errorf("got %v\nwant %v", len(requests), 1)
		}
		b, err := io.ReadAll(requests[0].Body)
		if err != nil {
			t.Fatal(err)
		}
		got := string(b)
		want := `{"jsonrpc":"2.0", "method": "subtract", "params": {"a": 10, "b": 3}, "id": 123}`
		if got != want {
			t.Errorf("got %v\nwant %v", got, want)
		}
	})
}

func TestMatcherMethod(t *testing.T) {
	rt := NewRouter(t)
	rt.Path("/api/v1/users/1").Method(http.MethodGet).ResponseString(http.StatusAccepted, `{"message":"accepted"}`)
	ts := rt.Server()
	t.Cleanup(func() {
		ts.Close()
	})
	tc := ts.Client()

	res, err := tc.Get("https://example.com/api/v1/users/1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		res.Body.Close()
	})

	got := res.StatusCode
	want := http.StatusAccepted
	if got != want {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestRouterQuery(t *testing.T) {
	rt := NewRouter(t)
	rt.Query("page", "3").Response(http.StatusOK, []byte(`{"data": []}`))
	ts := rt.Server()
	t.Cleanup(func() {
		ts.Close()
	})
	tc := ts.Client()

	res, err := tc.Get("https://example.com/api/v1/users?page=3")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		res.Body.Close()
	})

	got := res.StatusCode
	want := http.StatusOK
	if got != want {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestMatcherQuery(t *testing.T) {
	rt := NewRouter(t)
	rt.Path("/api/v1/users").Query("page", "3").Response(http.StatusOK, []byte(`{"data": []}`))
	ts := rt.Server()
	t.Cleanup(func() {
		ts.Close()
	})
	tc := ts.Client()

	res, err := tc.Get("https://example.com/api/v1/users?page=3")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		res.Body.Close()
	})

	got := res.StatusCode
	want := http.StatusOK
	if got != want {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestRouterDefaultHeader(t *testing.T) {
	rt := NewRouter(t)
	rt.DefaultHeader("Content-Type", "application/json")
	rt.Method(http.MethodGet).Path("/api/v1/users/1").ResponseString(http.StatusAccepted, `{"message":"accepted"}`)
	ts := rt.Server()
	t.Cleanup(func() {
		ts.Close()
	})
	tc := ts.Client()

	res, err := tc.Get("https://example.com/api/v1/users/1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		res.Body.Close()
	})

	got := res.Header.Get("Content-Type")
	want := "application/json"
	if got != want {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestRouterDefaultMiddleware(t *testing.T) {
	rt := NewRouter(t)
	rt.DefaultMiddleware(func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// override
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("{}"))
		}
	})
	rt.Method(http.MethodGet).Path("/api/v1/users/1").ResponseString(http.StatusAccepted, `{"message":"accepted"}`)
	ts := rt.Server()
	t.Cleanup(func() {
		ts.Close()
	})
	tc := ts.Client()

	res, err := tc.Get("https://example.com/api/v1/users/1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		res.Body.Close()
	})

	got := res.StatusCode
	want := http.StatusBadRequest
	if got != want {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestMatcherMiddleware(t *testing.T) {
	rt := NewRouter(t)
	rt.Method(http.MethodGet).Path("/api/v1/users/1").Middleware(func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// override
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("{}"))
		}
	}).ResponseString(http.StatusAccepted, `{"message":"accepted"}`)
	ts := rt.Server()
	t.Cleanup(func() {
		ts.Close()
	})
	tc := ts.Client()

	res, err := tc.Get("https://example.com/api/v1/users/1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		res.Body.Close()
	})

	got := res.StatusCode
	want := http.StatusForbidden
	if got != want {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestMatcherHander(t *testing.T) {
	rt := NewRouter(t)
	rt.Path("/api/v1/users/1").Method(http.MethodGet).Handler(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"message":"accepted"}`))
	})
	ts := rt.Server()
	t.Cleanup(func() {
		ts.Close()
	})
	tc := ts.Client()

	res, err := tc.Get("https://example.com/api/v1/users/1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		res.Body.Close()
	})

	got := res.StatusCode
	want := http.StatusAccepted
	if got != want {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestNewServer(t *testing.T) {
	ts := NewServer(t)
	ts.Method(http.MethodGet).Path("/api/v1/users/1").Header("Content-Type", "application/json").ResponseString(http.StatusOK, `{"name":"alice"}`)
	t.Cleanup(func() {
		ts.Close()
	})
	tc := ts.Client()

	res, err := tc.Get("https://example.com/api/v1/users/1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		res.Body.Close()
	})
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}

	got := string(body)
	want := `{"name":"alice"}`
	if got != want {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestRequests(t *testing.T) {
	rt := NewRouter(t)
	m := rt.Method(http.MethodGet).Path("/api/v1/users/1")
	m.Header("Content-Type", "application/json").ResponseString(http.StatusOK, `{"name":"alice"}`)
	rt.Method(http.MethodGet).Path("/api/v1/projects").Header("Content-Type", "application/json").ResponseString(http.StatusOK, `{"projects": []}`)
	ts := rt.Server()
	t.Cleanup(func() {
		ts.Close()
	})
	tc := ts.Client()

	res, err := tc.Get("https://example.com/api/v1/users/1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		res.Body.Close()
	})
	if want := 1; len(rt.Requests()) != want {
		t.Errorf("got %v\nwant %v", len(rt.Requests()), want)
	}
	if want := 1; len(m.Requests()) != want {
		t.Errorf("got %v\nwant %v", len(m.Requests()), want)
	}
}

func TestTLSServer(t *testing.T) {
	rt := NewRouter(t)
	rt.Method(http.MethodGet).Path("/api/v1/users/1").Header("Content-Type", "application/json").ResponseString(http.StatusOK, `{"name":"alice"}`)
	ts := rt.TLSServer()
	t.Cleanup(func() {
		ts.Close()
	})
	tc := ts.Client()
	res, err := tc.Get("https://example.com/api/v1/users/1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		res.Body.Close()
	})
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}

	{
		got := res.StatusCode
		want := http.StatusOK
		if got != want {
			t.Errorf("got %v\nwant %v", got, want)
		}
	}
	{
		got := res.Header.Get("Content-Type")
		want := "application/json"
		if got != want {
			t.Errorf("got %v\nwant %v", got, want)
		}
	}
	{
		got := string(body)
		want := `{"name":"alice"}`
		if got != want {
			t.Errorf("got %v\nwant %v", got, want)
		}
	}
}

func TestUseTLSWithCertificates(t *testing.T) {
	cacert, err := os.ReadFile("testdata/cacert.pem")
	if err != nil {
		t.Fatal(err)
	}
	cert, err := os.ReadFile("testdata/cert.pem")
	if err != nil {
		t.Fatal(err)
	}
	key, err := os.ReadFile("testdata/key.pem")
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRouter(t, UseTLSWithCertificates(cert, key), CACert(cacert))
	rt.Method(http.MethodGet).Path("/api/v1/users/1").Header("Content-Type", "application/json").ResponseString(http.StatusOK, `{"name":"alice"}`)
	ts := rt.Server()
	t.Cleanup(func() {
		ts.Close()
	})
	tc := ts.Client()
	res, err := tc.Get("http://example.com/api/v1/users/1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		res.Body.Close()
	})
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}

	{
		got := res.StatusCode
		want := http.StatusOK
		if got != want {
			t.Errorf("got %v\nwant %v", got, want)
		}
	}
	{
		got := res.Header.Get("Content-Type")
		want := "application/json"
		if got != want {
			t.Errorf("got %v\nwant %v", got, want)
		}
	}
	{
		got := string(body)
		want := `{"name":"alice"}`
		if got != want {
			t.Errorf("got %v\nwant %v", got, want)
		}
	}
}

func TestClientCertififaces(t *testing.T) {
	clientCacert, err := os.ReadFile("testdata/clientcacert.pem")
	if err != nil {
		t.Fatal(err)
	}
	clientCert, err := os.ReadFile("testdata/clientcert.pem")
	if err != nil {
		t.Fatal(err)
	}
	clientKey, err := os.ReadFile("testdata/clientkey.pem")
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRouter(t, UseTLS(), ClientCACert(clientCacert), ClientCertificates(clientCert, clientKey))
	rt.Method(http.MethodGet).Path("/api/v1/users/1").Header("Content-Type", "application/json").ResponseString(http.StatusOK, `{"name":"alice"}`)
	ts := rt.Server()
	t.Cleanup(func() {
		ts.Close()
	})
	tc := ts.Client()
	res, err := tc.Get("http://example.com/api/v1/users/1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		res.Body.Close()
	})
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}

	{
		got := res.StatusCode
		want := http.StatusOK
		if got != want {
			t.Errorf("got %v\nwant %v", got, want)
		}
	}
	{
		got := res.Header.Get("Content-Type")
		want := "application/json"
		if got != want {
			t.Errorf("got %v\nwant %v", got, want)
		}
	}
	{
		got := string(body)
		want := `{"name":"alice"}`
		if got != want {
			t.Errorf("got %v\nwant %v", got, want)
		}
	}
}

func TestClearRequests(t *testing.T) {
	tests := []struct {
		clearFunc func(*Router, *matcher)
		wantFunc  func(*testing.T, *Router, *matcher)
	}{
		{
			func(rt *Router, m *matcher) {
			},
			func(t *testing.T, rt *Router, m *matcher) {
				if want := 2; len(rt.Requests()) != want {
					t.Errorf("got %v\nwant %v", len(rt.Requests()), want)
				}
				if want := 1; len(m.Requests()) != want {
					t.Errorf("got %v\nwant %v", len(m.Requests()), want)
				}
			},
		},
		{
			func(rt *Router, m *matcher) {
				rt.ClearRequests()
			},
			func(t *testing.T, rt *Router, m *matcher) {
				if want := 0; len(rt.Requests()) != want {
					t.Errorf("got %v\nwant %v", len(rt.Requests()), want)
				}
				if want := 0; len(m.Requests()) != want {
					t.Errorf("got %v\nwant %v", len(m.Requests()), want)
				}
			},
		},
		{
			func(rt *Router, m *matcher) {
				m.ClearRequests()
			},
			func(t *testing.T, rt *Router, m *matcher) {
				if want := 1; len(rt.Requests()) != want {
					t.Errorf("got %v\nwant %v", len(rt.Requests()), want)
				}
				if want := 0; len(m.Requests()) != want {
					t.Errorf("got %v\nwant %v", len(m.Requests()), want)
				}
			},
		},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			rt := NewRouter(t)
			m := rt.Method(http.MethodGet).Path("/api/v1/users/1")
			m.Header("Content-Type", "application/json").ResponseString(http.StatusOK, `{"name":"alice"}`)
			m2 := rt.Method(http.MethodGet).Path("/api/v1/users/2")
			m2.Header("Content-Type", "application/json").ResponseString(http.StatusOK, `{"name":"bob"}`)
			rt.Method(http.MethodGet).Path("/api/v1/projects").Header("Content-Type", "application/json").ResponseString(http.StatusOK, `{"projects": []}`)
			ts := rt.Server()
			t.Cleanup(func() {
				ts.Close()
			})
			tc := ts.Client()

			res, err := tc.Get("https://example.com/api/v1/users/1")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				res.Body.Close()
			})
			res2, err := tc.Get("https://example.com/api/v1/users/2")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				res2.Body.Close()
			})
			tt.clearFunc(rt, m)
			tt.wantFunc(t, rt, m)
		})
	}
}

func TestMatcherResponseExample(t *testing.T) {
	tests := []struct {
		name            string
		req             *http.Request
		status          string
		wantContentType string
		wantErr         bool
	}{
		{"valid req/res", newRequest(t, http.MethodGet, "/api/v1/users", ""), "*", "application/json", false},
		{"valid req/res", newRequest(t, http.MethodGet, "/api/v1/ping", ""), "*", "text/plain", false},
		{"valid req/res with status 200", newRequest(t, http.MethodGet, "/api/v1/users", ""), "200", "application/json", false},
		{"valid req/res with status 2*", newRequest(t, http.MethodGet, "/api/v1/users", ""), "2*", "application/json", false},
		{"invalid req", newRequest(t, http.MethodPost, "/api/v1/users", `{"invalid": "alice", "req": "passw0rd"}`), "*", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockTB := mock_httpstub.NewMockTB(ctrl)
			mockTB.EXPECT().Helper()
			if tt.wantErr {
				mockTB.EXPECT().Errorf(gomock.Any(), gomock.Any())
			}
			rt := NewRouter(t, OpenApi3("testdata/openapi3.yml"))
			rt.t = mockTB
			rt.Method(http.MethodGet).Path("/api/v1/users").ResponseExample(Status(tt.status))
			rt.Method(http.MethodGet).Path("/api/v1/ping").ResponseExample(Status(tt.status))
			ts := rt.Server()
			t.Cleanup(func() {
				ts.Close()
			})
			tc := ts.Client()
			res, err := tc.Do(tt.req)
			if err != nil {
				t.Error(err)
				return
			}
			if tt.wantErr {
				return
			}
			got := res.Header.Get("Content-Type")
			if got != tt.wantContentType {
				t.Errorf("got %v\nwant %v", got, tt.wantContentType)
			}
		})
	}
}

func TestRouterResponseExample(t *testing.T) {
	tests := []struct {
		name            string
		req             *http.Request
		status          string
		wantContentType string
		wantErr         bool
	}{
		{"valid req/res", newRequest(t, http.MethodGet, "/api/v1/users", ""), "*", "application/json", false},
		{"valid req/res", newRequest(t, http.MethodGet, "/api/v1/ping", ""), "*", "text/plain", false},
		{"valid req/res with status 200", newRequest(t, http.MethodGet, "/api/v1/users", ""), "200", "application/json", false},
		{"valid req/res with status 2*", newRequest(t, http.MethodGet, "/api/v1/users", ""), "2*", "application/json", false},
		{"invalid req", newRequest(t, http.MethodPost, "/api/v1/users", `{"invalid": "alice", "req": "passw0rd"}`), "*", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockTB := mock_httpstub.NewMockTB(ctrl)
			mockTB.EXPECT().Helper()
			if tt.wantErr {
				mockTB.EXPECT().Errorf(gomock.Any(), gomock.Any())
			}
			rt := NewRouter(t, OpenApi3("testdata/openapi3.yml"))
			rt.t = mockTB
			rt.ResponseExample(Status(tt.status))
			ts := rt.Server()
			t.Cleanup(func() {
				ts.Close()
			})
			tc := ts.Client()
			res, err := tc.Do(tt.req)
			if err != nil {
				t.Error(err)
			}
			if tt.wantErr {
				return
			}
			got := res.Header.Get("Content-Type")
			if got != tt.wantContentType {
				t.Errorf("got %v\nwant %v", got, tt.wantContentType)
			}
		})
	}
}

func TestPrepend(t *testing.T) {
	t.Run("Default", func(t *testing.T) {
		rt := NewRouter(t)
		rt.Path("/api/v1/users/1").Response(http.StatusAccepted, []byte(`{"message":"accepted"}`))
		rt.Path("/api/v1/users/1").Response(http.StatusOK, []byte(`{"message":"ok"}`))
		ts := rt.Server()
		t.Cleanup(func() {
			ts.Close()
		})
		tc := ts.Client()

		res, err := tc.Get("https://example.com/api/v1/users/1")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			res.Body.Close()
		})

		got := res.StatusCode
		want := http.StatusAccepted
		if got != want {
			t.Errorf("got %v\nwant %v", got, want)
		}
	})

	t.Run("Prepend", func(t *testing.T) {
		rt := NewRouter(t)
		rt.Path("/api/v1/users/1").Response(http.StatusAccepted, []byte(`{"message":"accepted"}`))
		rt.Prepend().Path("/api/v1/users/1").Response(http.StatusOK, []byte(`{"message":"ok"}`))
		rt.Path("/api/v1/users/1").Response(http.StatusNotFound, []byte(`{"message":"not found"}`))
		ts := rt.Server()
		t.Cleanup(func() {
			ts.Close()
		})
		tc := ts.Client()

		res, err := tc.Get("https://example.com/api/v1/users/1")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			res.Body.Close()
		})

		got := res.StatusCode
		want := http.StatusOK
		if got != want {
			t.Errorf("got %v\nwant %v", got, want)
		}
	})
}

func TestURL(t *testing.T) {
	rt := NewRouter(t)
	{
		got := rt.URL
		if got != "" {
			t.Errorf("got %v want %v", got, "")
		}
	}
	ts := rt.Server()
	t.Cleanup(func() {
		ts.Close()
	})
	{
		got := rt.URL
		if got == "" {
			t.Error("want url")
		}
	}
}

func BenchmarkNewServer(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ts := NewServer(b)
		ts.Close()
	}
}
