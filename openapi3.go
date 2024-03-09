package httpstub

import (
	"bytes"
	"errors"
	"io"
	"net/http"
)

var _ http.ResponseWriter = (*recorder)(nil)

type recorder struct {
	rw         http.ResponseWriter
	statusCode int
	body       *bytes.Buffer
}

func newRecorder(rw http.ResponseWriter) *recorder {
	return &recorder{
		rw:   rw,
		body: bytes.NewBuffer(nil),
	}
}

func (r *recorder) Header() http.Header {
	return r.rw.Header()
}

func (r *recorder) Write(b []byte) (int, error) {
	if n, err := r.body.Write(b); err != nil {
		return n, err
	}
	return r.rw.Write(b)
}

func (r *recorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.rw.WriteHeader(statusCode)
}

func (r *recorder) toResponse() *http.Response {
	return &http.Response{
		Status:     http.StatusText(r.statusCode),
		StatusCode: r.statusCode,
		Body:       io.NopCloser(r.body),
		Header:     r.rw.Header().Clone(),
	}
}

func (rt *Router) setOpenApi3Vaildator() error {
	rt.t.Helper()
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.openAPIDoc == nil || rt.openAPIVersion != openAPIVersion3 {
		return nil
	}
	mw := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			v := *rt.openAPIValidator
			if !rt.skipValidateRequest {
				_, errs := v.ValidateHttpRequest(r)
				if len(errs) > 0 {
					var err error
					for _, e := range errs {
						err = errors.Join(err, e)
					}
					rt.t.Errorf("failed to validate response: %v", err)
				}
			}
			rec := newRecorder(w)
			next.ServeHTTP(rec, r)

			if !rt.skipValidateResponse {
				_, errs := v.ValidateHttpResponse(r, rec.toResponse())
				if len(errs) > 0 {
					var err error
					for _, e := range errs {
						err = errors.Join(err, e)
					}
					rt.t.Errorf("failed to validate response: %v", err)
				}
			}
		}
	}
	rt.middlewares = append(rt.middlewares, mw)
	return nil
}
