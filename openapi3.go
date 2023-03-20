package httpstub

import (
	"bytes"
	"context"
	"io"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3filter"
	legacyrouter "github.com/getkin/kin-openapi/routers/legacy"
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

func (rt *Router) setOpenApi3Vaildator() error {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.openApi3Doc == nil {
		return nil
	}
	router, err := legacyrouter.NewRouter(rt.openApi3Doc)
	if err != nil {
		return err
	}
	mw := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ctx := context.Background()
			var reqv *openapi3filter.RequestValidationInput
			route, pathParams, err := router.FindRoute(r)
			if err != nil {
				rt.t.Errorf("failed to find route: %v", err)
			} else {
				reqv = &openapi3filter.RequestValidationInput{
					Request:    r,
					PathParams: pathParams,
					Route:      route,
					Options: &openapi3filter.Options{
						AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
					},
				}
				if !rt.skipValidateRequest {
					if err := openapi3filter.ValidateRequest(ctx, reqv); err != nil {
						rt.t.Errorf("failed to validate request: %v", err)
					}
				}
			}
			rec := newRecorder(w)
			next.ServeHTTP(rec, r)
			if reqv != nil {
				resv := &openapi3filter.ResponseValidationInput{
					RequestValidationInput: reqv,
					Status:                 rec.statusCode,
					Header:                 w.Header(),
					Body:                   io.NopCloser(rec.body),
					Options: &openapi3filter.Options{
						IncludeResponseStatus: true,
					},
				}
				if !rt.skipValidateResponse {
					if err := openapi3filter.ValidateResponse(ctx, resv); err != nil {
						rt.t.Errorf("failed to validate response: %v", err)
					}
				}
			}
		}
	}
	rt.middlewares = append(rt.middlewares, mw)
	return nil
}
