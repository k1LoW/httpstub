package httpstub

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	mock_httpstub "github.com/k1LoW/httpstub/mock"
)

func TestResponseDynamic_BasicTypes(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
		validator      func(t *testing.T, body []byte)
	}{
		{
			name:           "string basic GET",
			method:         http.MethodGet,
			path:           "/string-basic",
			expectedStatus: http.StatusOK,
			validator: func(t *testing.T, body []byte) {
				var result string
				if err := json.Unmarshal(body, &result); err != nil {
					t.Errorf("failed to unmarshal: %v", err)
				}
				if result == "" {
					t.Error("expected non-empty string")
				}
			},
		},
		{
			name:           "string basic POST",
			method:         http.MethodPost,
			path:           "/string-basic",
			expectedStatus: http.StatusCreated,
			validator: func(t *testing.T, body []byte) {
				var result string
				if err := json.Unmarshal(body, &result); err != nil {
					t.Errorf("failed to unmarshal: %v", err)
				}
				if result == "" {
					t.Error("expected non-empty string")
				}
			},
		},
		{
			name:           "string basic DELETE",
			method:         http.MethodDelete,
			path:           "/string-basic",
			expectedStatus: http.StatusNoContent,
			validator: func(t *testing.T, body []byte) {
				if len(body) != 0 {
					t.Errorf("expected empty body for 204, got %d bytes", len(body))
				}
			},
		},
		{
			name:           "number basic GET",
			method:         http.MethodGet,
			path:           "/number-basic",
			expectedStatus: http.StatusOK,
			validator: func(t *testing.T, body []byte) {
				var result float64
				if err := json.Unmarshal(body, &result); err != nil {
					t.Errorf("failed to unmarshal: %v", err)
				}
			},
		},
		{
			name:           "number basic POST",
			method:         http.MethodPost,
			path:           "/number-basic",
			expectedStatus: http.StatusCreated,
			validator: func(t *testing.T, body []byte) {
				var result float64
				if err := json.Unmarshal(body, &result); err != nil {
					t.Errorf("failed to unmarshal: %v", err)
				}
			},
		},
		{
			name:           "number basic DELETE",
			method:         http.MethodDelete,
			path:           "/number-basic",
			expectedStatus: http.StatusNoContent,
			validator: func(t *testing.T, body []byte) {
				if len(body) != 0 {
					t.Errorf("expected empty body for 204, got %d bytes", len(body))
				}
			},
		},
		{
			name:           "integer basic GET",
			method:         http.MethodGet,
			path:           "/integer-basic",
			expectedStatus: http.StatusOK,
			validator: func(t *testing.T, body []byte) {
				var result int64
				if err := json.Unmarshal(body, &result); err != nil {
					t.Errorf("failed to unmarshal: %v", err)
				}
			},
		},
		{
			name:           "integer basic POST",
			method:         http.MethodPost,
			path:           "/integer-basic",
			expectedStatus: http.StatusCreated,
			validator: func(t *testing.T, body []byte) {
				var result int64
				if err := json.Unmarshal(body, &result); err != nil {
					t.Errorf("failed to unmarshal: %v", err)
				}
			},
		},
		{
			name:           "integer basic DELETE",
			method:         http.MethodDelete,
			path:           "/integer-basic",
			expectedStatus: http.StatusNoContent,
			validator: func(t *testing.T, body []byte) {
				if len(body) != 0 {
					t.Errorf("expected empty body for 204, got %d bytes", len(body))
				}
			},
		},
		{
			name:           "boolean basic GET",
			method:         http.MethodGet,
			path:           "/boolean-basic",
			expectedStatus: http.StatusOK,
			validator: func(t *testing.T, body []byte) {
				var result bool
				if err := json.Unmarshal(body, &result); err != nil {
					t.Errorf("failed to unmarshal: %v", err)
				}
			},
		},
		{
			name:           "boolean basic POST",
			method:         http.MethodPost,
			path:           "/boolean-basic",
			expectedStatus: http.StatusCreated,
			validator: func(t *testing.T, body []byte) {
				var result bool
				if err := json.Unmarshal(body, &result); err != nil {
					t.Errorf("failed to unmarshal: %v", err)
				}
			},
		},
		{
			name:           "boolean basic DELETE",
			method:         http.MethodDelete,
			path:           "/boolean-basic",
			expectedStatus: http.StatusNoContent,
			validator: func(t *testing.T, body []byte) {
				if len(body) != 0 {
					t.Errorf("expected empty body for 204, got %d bytes", len(body))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockTB := mock_httpstub.NewMockTB(ctrl)
			mockTB.EXPECT().Helper().AnyTimes()

			rt := NewRouter(mockTB, OpenApi3("testdata/openapi3-dynamic-test.yml"))
			rt.Method(tt.method).Path(tt.path).ResponseDynamic()

			ts := rt.Server()
			t.Cleanup(func() {
				ts.Close()
			})

			tc := ts.Client()
			req := newRequest(t, tt.method, tt.path, "")
			res, err := tc.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()

			if res.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, res.StatusCode)
			}

			body, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatal(err)
			}

			tt.validator(t, body)
		})
	}
}

func TestResponseDynamic_StringConstraints(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		validator func(t *testing.T, body []byte)
	}{
		{
			name: "string with length constraints",
			path: "/string-with-length",
			validator: func(t *testing.T, body []byte) {
				var result string
				if err := json.Unmarshal(body, &result); err != nil {
					t.Errorf("failed to unmarshal: %v", err)
				}
				length := len(result)
				if length < 5 || length > 10 {
					t.Errorf("expected length between 5 and 10, got %d", length)
				}
			},
		},
		{
			name: "string with enum",
			path: "/string-with-enum",
			validator: func(t *testing.T, body []byte) {
				var result string
				if err := json.Unmarshal(body, &result); err != nil {
					t.Errorf("failed to unmarshal: %v", err)
				}
				validValues := map[string]bool{"active": true, "inactive": true, "pending": true}
				if !validValues[result] {
					t.Errorf("expected one of [active, inactive, pending], got %s", result)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockTB := mock_httpstub.NewMockTB(ctrl)
			mockTB.EXPECT().Helper().AnyTimes()

			rt := NewRouter(mockTB, OpenApi3("testdata/openapi3-dynamic-test.yml"))
			rt.Method(http.MethodGet).Path(tt.path).ResponseDynamic()

			ts := rt.Server()
			t.Cleanup(func() {
				ts.Close()
			})

			tc := ts.Client()
			req := newRequest(t, http.MethodGet, tt.path, "")
			res, err := tc.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()

			body, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatal(err)
			}

			tt.validator(t, body)
		})
	}
}

func TestResponseDynamic_StringFormats(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		validator func(t *testing.T, body []byte)
	}{
		{
			name: "date format",
			path: "/string-with-format-date",
			validator: func(t *testing.T, body []byte) {
				var result string
				if err := json.Unmarshal(body, &result); err != nil {
					t.Errorf("failed to unmarshal: %v", err)
				}
				// YYYY-MM-DD format
				matched, _ := regexp.MatchString(`^\d{4}-\d{2}-\d{2}$`, result)
				if !matched {
					t.Errorf("expected date format (YYYY-MM-DD), got %s", result)
				}
			},
		},
		{
			name: "date-time format",
			path: "/string-with-format-datetime",
			validator: func(t *testing.T, body []byte) {
				var result string
				if err := json.Unmarshal(body, &result); err != nil {
					t.Errorf("failed to unmarshal: %v", err)
				}
				// RFC3339 format
				matched, _ := regexp.MatchString(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`, result)
				if !matched {
					t.Errorf("expected date-time format (RFC3339), got %s", result)
				}
			},
		},
		{
			name: "email format",
			path: "/string-with-format-email",
			validator: func(t *testing.T, body []byte) {
				var result string
				if err := json.Unmarshal(body, &result); err != nil {
					t.Errorf("failed to unmarshal: %v", err)
				}
				matched, _ := regexp.MatchString(`^[^@]+@[^@]+\.[^@]+$`, result)
				if !matched {
					t.Errorf("expected email format, got %s", result)
				}
			},
		},
		{
			name: "uuid format",
			path: "/string-with-format-uuid",
			validator: func(t *testing.T, body []byte) {
				var result string
				if err := json.Unmarshal(body, &result); err != nil {
					t.Errorf("failed to unmarshal: %v", err)
				}
				matched, _ := regexp.MatchString(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, result)
				if !matched {
					t.Errorf("expected uuid format, got %s", result)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockTB := mock_httpstub.NewMockTB(ctrl)
			mockTB.EXPECT().Helper().AnyTimes()

			rt := NewRouter(mockTB, OpenApi3("testdata/openapi3-dynamic-test.yml"))
			rt.Method(http.MethodGet).Path(tt.path).ResponseDynamic()

			ts := rt.Server()
			t.Cleanup(func() {
				ts.Close()
			})

			tc := ts.Client()
			req := newRequest(t, http.MethodGet, tt.path, "")
			res, err := tc.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()

			body, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatal(err)
			}

			tt.validator(t, body)
		})
	}
}

func TestResponseDynamic_NumberConstraints(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		validator func(t *testing.T, body []byte)
	}{
		{
			name: "number with range",
			path: "/number-with-range",
			validator: func(t *testing.T, body []byte) {
				var result float64
				if err := json.Unmarshal(body, &result); err != nil {
					t.Errorf("failed to unmarshal: %v", err)
				}
				if result < 10.5 || result > 99.9 {
					t.Errorf("expected number between 10.5 and 99.9, got %f", result)
				}
			},
		},
		{
			name: "number with enum",
			path: "/number-with-enum",
			validator: func(t *testing.T, body []byte) {
				var result float64
				if err := json.Unmarshal(body, &result); err != nil {
					t.Errorf("failed to unmarshal: %v", err)
				}
				validValues := map[float64]bool{1.5: true, 2.5: true, 3.5: true}
				if !validValues[result] {
					t.Errorf("expected one of [1.5, 2.5, 3.5], got %f", result)
				}
			},
		},
		{
			name: "integer with range",
			path: "/integer-with-range",
			validator: func(t *testing.T, body []byte) {
				var result int64
				if err := json.Unmarshal(body, &result); err != nil {
					t.Errorf("failed to unmarshal: %v", err)
				}
				if result < 1 || result > 100 {
					t.Errorf("expected integer between 1 and 100, got %d", result)
				}
			},
		},
		{
			name: "integer with enum",
			path: "/integer-with-enum",
			validator: func(t *testing.T, body []byte) {
				var result int64
				if err := json.Unmarshal(body, &result); err != nil {
					t.Errorf("failed to unmarshal: %v", err)
				}
				validValues := map[int64]bool{1: true, 2: true, 3: true, 5: true, 8: true}
				if !validValues[result] {
					t.Errorf("expected one of [1, 2, 3, 5, 8], got %d", result)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockTB := mock_httpstub.NewMockTB(ctrl)
			mockTB.EXPECT().Helper().AnyTimes()

			rt := NewRouter(mockTB, OpenApi3("testdata/openapi3-dynamic-test.yml"))
			rt.Method(http.MethodGet).Path(tt.path).ResponseDynamic()

			ts := rt.Server()
			t.Cleanup(func() {
				ts.Close()
			})

			tc := ts.Client()
			req := newRequest(t, http.MethodGet, tt.path, "")
			res, err := tc.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()

			body, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatal(err)
			}

			tt.validator(t, body)
		})
	}
}

func TestResponseDynamic_Arrays(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		validator func(t *testing.T, body []byte)
	}{
		{
			name: "array of strings",
			path: "/array-of-strings",
			validator: func(t *testing.T, body []byte) {
				var result []string
				if err := json.Unmarshal(body, &result); err != nil {
					t.Errorf("failed to unmarshal: %v", err)
				}
				if len(result) < 1 || len(result) > 5 {
					t.Errorf("expected array length between 1 and 5, got %d", len(result))
				}
			},
		},
		{
			name: "array of objects",
			path: "/array-of-objects",
			validator: func(t *testing.T, body []byte) {
				var result []map[string]interface{}
				if err := json.Unmarshal(body, &result); err != nil {
					t.Errorf("failed to unmarshal: %v", err)
				}
				for _, obj := range result {
					if _, ok := obj["id"]; !ok {
						t.Error("expected 'id' field in object (required)")
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockTB := mock_httpstub.NewMockTB(ctrl)
			mockTB.EXPECT().Helper().AnyTimes()

			rt := NewRouter(mockTB, OpenApi3("testdata/openapi3-dynamic-test.yml"))
			rt.Method(http.MethodGet).Path(tt.path).ResponseDynamic()

			ts := rt.Server()
			t.Cleanup(func() {
				ts.Close()
			})

			tc := ts.Client()
			req := newRequest(t, http.MethodGet, tt.path, "")
			res, err := tc.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()

			body, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatal(err)
			}

			tt.validator(t, body)
		})
	}
}

func TestResponseDynamic_Objects(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		validator func(t *testing.T, body []byte)
	}{
		{
			name: "simple object",
			path: "/object-simple",
			validator: func(t *testing.T, body []byte) {
				var result map[string]interface{}
				if err := json.Unmarshal(body, &result); err != nil {
					t.Errorf("failed to unmarshal: %v", err)
				}
				if _, ok := result["username"]; !ok {
					t.Error("expected 'username' field (required)")
				}
			},
		},
		{
			name: "nested object",
			path: "/object-nested",
			validator: func(t *testing.T, body []byte) {
				var result map[string]interface{}
				if err := json.Unmarshal(body, &result); err != nil {
					t.Errorf("failed to unmarshal: %v", err)
				}
				if _, ok := result["user"]; !ok {
					t.Error("expected 'user' field (required)")
				}
				if user, ok := result["user"].(map[string]interface{}); ok {
					if _, ok := user["name"]; !ok {
						t.Error("expected 'name' field in user (required)")
					}
					if address, ok := user["address"].(map[string]interface{}); ok {
						if _, ok := address["city"]; !ok {
							t.Error("expected 'city' field in address (required)")
						}
					}
				}
			},
		},
		{
			name: "object with nullable",
			path: "/object-with-nullable",
			validator: func(t *testing.T, body []byte) {
				var result map[string]interface{}
				if err := json.Unmarshal(body, &result); err != nil {
					t.Errorf("failed to unmarshal: %v", err)
				}
				if _, ok := result["name"]; !ok {
					t.Error("expected 'name' field (required)")
				}
				// email and phone can be null or string
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockTB := mock_httpstub.NewMockTB(ctrl)
			mockTB.EXPECT().Helper().AnyTimes()

			rt := NewRouter(mockTB, OpenApi3("testdata/openapi3-dynamic-test.yml"))
			rt.Method(http.MethodGet).Path(tt.path).ResponseDynamic()

			ts := rt.Server()
			t.Cleanup(func() {
				ts.Close()
			})

			tc := ts.Client()
			req := newRequest(t, http.MethodGet, tt.path, "")
			res, err := tc.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()

			body, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatal(err)
			}

			tt.validator(t, body)
		})
	}
}

func TestResponseDynamic_ComplexSchemas(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		validator func(t *testing.T, body []byte)
	}{
		{
			name: "allOf schema",
			path: "/object-with-allof",
			validator: func(t *testing.T, body []byte) {
				var result map[string]interface{}
				if err := json.Unmarshal(body, &result); err != nil {
					t.Errorf("failed to unmarshal: %v", err)
				}
				if _, ok := result["id"]; !ok {
					t.Error("expected 'id' field (required)")
				}
			},
		},
		{
			name: "anyOf schema",
			path: "/object-with-anyof",
			validator: func(t *testing.T, body []byte) {
				var result map[string]interface{}
				if err := json.Unmarshal(body, &result); err != nil {
					t.Errorf("failed to unmarshal: %v", err)
				}
				// Should generate using the first schema in anyOf
			},
		},
		{
			name: "oneOf schema",
			path: "/object-with-oneof",
			validator: func(t *testing.T, body []byte) {
				var result map[string]interface{}
				if err := json.Unmarshal(body, &result); err != nil {
					t.Errorf("failed to unmarshal: %v", err)
				}
				// Should generate using the first schema in oneOf
			},
		},
		{
			name: "complex schema",
			path: "/complex-schema",
			validator: func(t *testing.T, body []byte) {
				var result map[string]interface{}
				if err := json.Unmarshal(body, &result); err != nil {
					t.Errorf("failed to unmarshal: %v", err)
				}
				// Check required fields
				requiredFields := []string{"id", "name", "email", "status"}
				for _, field := range requiredFields {
					if _, ok := result[field]; !ok {
						t.Errorf("expected required field '%s'", field)
					}
				}
				// Check enum value
				if status, ok := result["status"].(string); ok {
					validStatus := map[string]bool{"active": true, "inactive": true, "suspended": true}
					if !validStatus[status] {
						t.Errorf("status should be one of [active, inactive, suspended], got %s", status)
					}
				}
				// Check email format
				if email, ok := result["email"].(string); ok {
					if !strings.Contains(email, "@") {
						t.Errorf("email should be in email format, got %s", email)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockTB := mock_httpstub.NewMockTB(ctrl)
			mockTB.EXPECT().Helper().AnyTimes()

			rt := NewRouter(mockTB, OpenApi3("testdata/openapi3-dynamic-test.yml"))
			rt.Method(http.MethodGet).Path(tt.path).ResponseDynamic()

			ts := rt.Server()
			t.Cleanup(func() {
				ts.Close()
			})

			tc := ts.Client()
			req := newRequest(t, http.MethodGet, tt.path, "")
			res, err := tc.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()

			body, err := io.ReadAll(res.Body)
			if err != nil {
				t.Fatal(err)
			}

			tt.validator(t, body)
		})
	}
}

func TestResponseDynamic_TextPlain(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockTB := mock_httpstub.NewMockTB(ctrl)
	mockTB.EXPECT().Helper().AnyTimes()

	rt := NewRouter(mockTB, OpenApi3("testdata/openapi3-dynamic-test.yml"))
	rt.Method(http.MethodGet).Path("/text-plain").ResponseDynamic()

	ts := rt.Server()
	t.Cleanup(func() {
		ts.Close()
	})

	tc := ts.Client()
	req := newRequest(t, http.MethodGet, "/text-plain", "")
	res, err := tc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", res.StatusCode)
	}

	contentType := res.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/plain") {
		t.Errorf("expected Content-Type to contain 'text/plain', got %s", contentType)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}

	if len(body) == 0 {
		t.Error("expected non-empty body")
	}
}

func TestResponseDynamic_NoContent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockTB := mock_httpstub.NewMockTB(ctrl)
	mockTB.EXPECT().Helper().AnyTimes()

	rt := NewRouter(mockTB, OpenApi3("testdata/openapi3-dynamic-test.yml"))
	rt.Method(http.MethodGet).Path("/no-content").ResponseDynamic()

	ts := rt.Server()
	t.Cleanup(func() {
		ts.Close()
	})

	tc := ts.Client()
	req := newRequest(t, http.MethodGet, "/no-content", "")
	res, err := tc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		t.Errorf("expected status 204, got %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}

	if len(body) != 0 {
		t.Errorf("expected empty body for 204, got %d bytes", len(body))
	}
}

func TestResponseDynamic_WithStatusOption(t *testing.T) {
	tests := []struct {
		name           string
		statusPattern  string
		expectedStatus int
	}{
		{
			name:           "specific status 200",
			statusPattern:  "200",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "specific status 201",
			statusPattern:  "201",
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "specific status 400",
			statusPattern:  "400",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockTB := mock_httpstub.NewMockTB(ctrl)
			mockTB.EXPECT().Helper().AnyTimes()

			rt := NewRouter(mockTB, OpenApi3("testdata/openapi3-dynamic-test.yml"))
			rt.Method(http.MethodGet).Path("/multiple-status").ResponseDynamic(Status(tt.statusPattern))

			ts := rt.Server()
			t.Cleanup(func() {
				ts.Close()
			})

			tc := ts.Client()
			req := newRequest(t, http.MethodGet, "/multiple-status", "")
			res, err := tc.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()

			if res.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, res.StatusCode)
			}
		})
	}
}

func TestResponseDynamic_RouterDefault(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockTB := mock_httpstub.NewMockTB(ctrl)
	mockTB.EXPECT().Helper().AnyTimes()

	rt := NewRouter(mockTB, OpenApi3("testdata/openapi3-dynamic-test.yml"))
	// Use Router.ResponseDynamic as default handler
	rt.ResponseDynamic()

	ts := rt.Server()
	t.Cleanup(func() {
		ts.Close()
	})

	tc := ts.Client()
	req := newRequest(t, http.MethodGet, "/string-basic", "")
	res, err := tc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}

	var result string
	if err := json.Unmarshal(body, &result); err != nil {
		t.Errorf("failed to unmarshal: %v", err)
	}

	if result == "" {
		t.Error("expected non-empty string")
	}
}

func TestResponseDynamic_ErrorNoOpenAPIDoc(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockTB := mock_httpstub.NewMockTB(ctrl)
	mockTB.EXPECT().Helper().AnyTimes()
	mockTB.EXPECT().Fatal(gomock.Any())

	rt := NewRouter(mockTB) // No OpenAPI document
	rt.Method(http.MethodGet).Path("/test").ResponseDynamic()
}

func TestResponseDynamic_ErrorInvalidRoute(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockTB := mock_httpstub.NewMockTB(ctrl)
	mockTB.EXPECT().Helper().AnyTimes()
	mockTB.EXPECT().Errorf(gomock.Any(), gomock.Any()).MinTimes(1)

	rt := NewRouter(mockTB, OpenApi3("testdata/openapi3-dynamic-test.yml"))
	rt.Method(http.MethodGet).Path("/nonexistent").ResponseDynamic()

	ts := rt.Server()
	t.Cleanup(func() {
		ts.Close()
	})

	tc := ts.Client()
	req := newRequest(t, http.MethodGet, "/nonexistent", "")
	_, _ = tc.Do(req)
}

func TestResponseDynamic_MultipleRuns(t *testing.T) {
	// Test that multiple runs generate valid but potentially different data
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockTB := mock_httpstub.NewMockTB(ctrl)
	mockTB.EXPECT().Helper().AnyTimes()

	rt := NewRouter(mockTB, OpenApi3("testdata/openapi3-dynamic-test.yml"))
	rt.Method(http.MethodGet).Path("/integer-with-range").ResponseDynamic()

	ts := rt.Server()
	t.Cleanup(func() {
		ts.Close()
	})

	tc := ts.Client()

	// Run multiple times and collect results
	results := make(map[int64]bool)
	for i := 0; i < 10; i++ {
		req := newRequest(t, http.MethodGet, "/integer-with-range", "")
		res, err := tc.Do(req)
		if err != nil {
			t.Fatal(err)
		}

		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			t.Fatal(err)
		}

		var result int64
		if err := json.Unmarshal(body, &result); err != nil {
			t.Errorf("failed to unmarshal: %v", err)
		}

		if result < 1 || result > 100 {
			t.Errorf("expected integer between 1 and 100, got %d", result)
		}

		results[result] = true
	}

	// We expect at least some variation in the results (though not guaranteed)
	// At minimum, all results should be valid
	if len(results) == 0 {
		t.Error("expected at least one valid result")
	}
}
