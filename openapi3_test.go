package httpstub

import (
	"net/http"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	mock_httpstub "github.com/k1LoW/httpstub/mock"
)

func TestOpenAPI3(t *testing.T) {
	tests := []struct {
		name    string
		req     *http.Request
		wantErr bool
	}{
		{"valid req/res", newRequest(t, http.MethodPost, "/api/v1/users", `{"username": "alice", "password": "passw0rd"}`), false},
		{"invalid route", newRequest(t, http.MethodPost, "/api/v1/invalid/route", `{"username": "alice", "password": "passw0rd"}`), true},
		{"invalid req", newRequest(t, http.MethodPost, "/api/v1/users", `{"invalid": "alice", "req": "passw0rd"}`), true},
		{"invalid res", newRequest(t, http.MethodGet, "/api/v1/users", ``), true},
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
			rt.Method(http.MethodPost).Path("/users").Header("Content-Type", "application/json").ResponseString(http.StatusCreated, `{"name":"alice"}`)
			// invalid response
			rt.Method(http.MethodGet).Path("/users").Header("Content-Type", "application/json").ResponseString(http.StatusBadRequest, `{"invalid":"data"}`)
			ts := rt.Server()
			t.Cleanup(func() {
				ts.Close()
			})
			tc := ts.Client()
			if _, err := tc.Do(tt.req); err != nil {
				t.Error(err)
			}
		})
	}
}

func newRequest(t *testing.T, method string, path string, body string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Add("Content-Type", "application/json")
	return req
}
