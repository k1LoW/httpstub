package httpstub

import (
	"bytes"
	"errors"
	"io"
	"net/http"

	validator "github.com/pb33f/libopenapi-validator"
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
	if rt.openAPI3Doc == nil {
		return nil
	}
	mw := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			v := rt.openAPI3Validator
			if !rt.skipValidateRequest {
				_, errs := v.ValidateHttpRequest(r)
				if len(errs) > 0 {
					{
						// renew validator (workaround)
						// ref: https://github.com/k1LoW/runn/issues/882
						vv, errrs := validator.NewValidator(rt.openAPI3Doc)
						if len(errrs) > 0 {
							rt.t.Errorf("failed to renew validator: %v", errors.Join(errrs...))
							return
						}
						rt.openAPI3Validator = vv
						v = rt.openAPI3Validator
					}
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
					{
						// renew validator (workaround)
						// ref: https://github.com/k1LoW/runn/issues/882
						vv, errrs := validator.NewValidator(rt.openAPI3Doc)
						if len(errrs) > 0 {
							rt.t.Errorf("failed to renew validator: %v", errors.Join(errrs...))
							return
						}
						rt.openAPI3Validator = vv
					}
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
