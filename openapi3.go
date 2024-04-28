package httpstub

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	verrors "github.com/pb33f/libopenapi-validator/errors"
	"gopkg.in/yaml.v3"
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
			v := *rt.openAPI3Validator
			if !rt.skipValidateRequest {
				_, errs := v.ValidateHttpRequest(r)
				if len(errs) > 0 {
					var err error
					for _, e := range errs {
						// nullable type workaround.
						if nullableError(e) {
							continue
						}
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
						// nullable type workaround.
						if nullableError(e) {
							continue
						}
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

// nullableTypeError returns whether the error is nullable type error or not.
func nullableError(e *verrors.ValidationError) bool {
	if len(e.SchemaValidationErrors) > 0 {
		for _, ve := range e.SchemaValidationErrors {
			if strings.HasSuffix(ve.Reason, "but got null") && strings.HasSuffix(ve.Location, "/type") {
				if nullableType(ve.ReferenceSchema, ve.Location) {
					return true
				}
			}
		}
	}
	return false
}

// nullableType returns whether the type is nullable or not.
func nullableType(schema, location string) bool {
	splitted := strings.Split(strings.TrimPrefix(strings.TrimSuffix(location, "/type")+"/nullable", "/"), "/")
	m := map[string]any{}
	if err := yaml.Unmarshal([]byte(schema), &m); err != nil {
		return false
	}
	v, ok := valueWithKeys(m, splitted...)
	if !ok {
		return false
	}
	if tf, ok := v.(bool); ok {
		return tf
	}
	return false
}

func valueWithKeys(m any, keys ...string) (any, bool) {
	if len(keys) == 0 {
		return nil, false
	}
	switch m := m.(type) {
	case map[string]any:
		if v, ok := m[keys[0]]; ok {
			if len(keys) == 1 {
				return v, true
			}
			return valueWithKeys(v, keys[1:]...)
		}
	case []any:
		i, err := strconv.Atoi(keys[0])
		if err != nil {
			return nil, false
		}
		if i < 0 || i >= len(m) {
			return nil, false
		}
		v := m[i]
		if len(keys) == 1 {
			return v, true
		}
		return valueWithKeys(v, keys[1:]...)
	default:
		return nil, false
	}
	return nil, false
}
