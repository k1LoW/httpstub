package httpstub

import (
	"io"
	"net/http"
	"os"
	"testing"
)

func TestStub(t *testing.T) {
	r := NewRouter(t)
	r.Method(http.MethodGet).Path("/api/v1/users/1").Header("Content-Type", "application/json").ResponseString(http.StatusOK, `{"name":"alice"}`)
	ts := r.Server()
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

func TestRouterMatch(t *testing.T) {
	r := NewRouter(t)
	r.Match(func(r *http.Request) bool {
		return r.Method == http.MethodGet
	}).Response(http.StatusAccepted, []byte(`{"message":"accepted"}`))
	ts := r.Server()
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
	r := NewRouter(t)
	r.Path("/api/v1/users/1").Match(func(r *http.Request) bool {
		return r.Method == http.MethodGet
	}).ResponseString(http.StatusAccepted, `{"message":"accepted"}`)
	ts := r.Server()
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

func TestMatcherMethod(t *testing.T) {
	r := NewRouter(t)
	r.Path("/api/v1/users/1").Method(http.MethodGet).ResponseString(http.StatusAccepted, `{"message":"accepted"}`)
	ts := r.Server()
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

func TestRouterDefaultHeader(t *testing.T) {
	r := NewRouter(t)
	r.DefaultHeader("Content-Type", "application/json")
	r.Method(http.MethodGet).Path("/api/v1/users/1").ResponseString(http.StatusAccepted, `{"message":"accepted"}`)
	ts := r.Server()
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
	r := NewRouter(t)
	r.DefaultMiddleware(func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// override
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("{}"))
		}
	})
	r.Method(http.MethodGet).Path("/api/v1/users/1").ResponseString(http.StatusAccepted, `{"message":"accepted"}`)
	ts := r.Server()
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
	r := NewRouter(t)
	r.Method(http.MethodGet).Path("/api/v1/users/1").Middleware(func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// override
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("{}"))
		}
	}).ResponseString(http.StatusAccepted, `{"message":"accepted"}`)
	ts := r.Server()
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
	r := NewRouter(t)
	r.Path("/api/v1/users/1").Method(http.MethodGet).Handler(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"message":"accepted"}`))
	})
	ts := r.Server()
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
	r := NewRouter(t)
	m := r.Method(http.MethodGet).Path("/api/v1/users/1")
	m.Header("Content-Type", "application/json").ResponseString(http.StatusOK, `{"name":"alice"}`)
	r.Method(http.MethodGet).Path("/api/v1/projects").Header("Content-Type", "application/json").ResponseString(http.StatusOK, `{"projects": []}`)
	ts := r.Server()
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
	if len(r.Requests()) != 1 {
		t.Errorf("got %v\nwant %v", len(r.Requests()), 1)
	}
	if len(m.Requests()) != 1 {
		t.Errorf("got %v\nwant %v", len(m.Requests()), 1)
	}
}

func TestTLSServer(t *testing.T) {
	r := NewRouter(t)
	r.Method(http.MethodGet).Path("/api/v1/users/1").Header("Content-Type", "application/json").ResponseString(http.StatusOK, `{"name":"alice"}`)
	ts := r.TLSServer()
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
	r := NewRouter(t, UseTLSWithCertificates(cert, key), CACert(cacert))
	r.Method(http.MethodGet).Path("/api/v1/users/1").Header("Content-Type", "application/json").ResponseString(http.StatusOK, `{"name":"alice"}`)
	ts := r.Server()
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
