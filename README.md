# httpstub [![Go Reference](https://pkg.go.dev/badge/github.com/k1LoW/httpstub.svg)](https://pkg.go.dev/github.com/k1LoW/httpstub) ![Coverage](https://raw.githubusercontent.com/k1LoW/octocovs/main/badges/k1LoW/httpstub/coverage.svg) ![Code to Test Ratio](https://raw.githubusercontent.com/k1LoW/octocovs/main/badges/k1LoW/httpstub/ratio.svg) ![Test Execution Time](https://raw.githubusercontent.com/k1LoW/octocovs/main/badges/k1LoW/httpstub/time.svg)

httpstub provides router ( `http.Handler` ), server ( `*httptest.Server` ) and client ( `*http.Client` ) for stubbing, for testing in Go.

There is an gRPC version stubbing tool with the same design concept, [grpcstub](https://github.com/k1LoW/grpcstub).

## Usage

``` go
package myapp

import (
	"io"
	"net/http"
	"testing"

	"github.com/k1LoW/httpstub"
)

func TestGet(t *testing.T) {
	ts := httpstub.NewServer(t)
	t.Cleanup(func() {
		ts.Close()
	})
	ts.Method(http.MethodGet).Path("/api/v1/users/1").Header("Content-Type", "application/json").ResponseString(http.StatusOK, `{"name":"alice"}`)

	res, err := http.Get(ts.URL + "/api/v1/users/1")
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
	if len(ts.Requests()) != 1 {
		t.Errorf("got %v\nwant %v", len(ts.Requests()), 1)
	}
}
```

or

``` go
package myapp

import (
	"io"
	"net/http"
	"testing"

	"github.com/k1LoW/httpstub"
)

func TestGet(t *testing.T) {
	r := httpstub.NewRouter(t)
	r.Method(http.MethodGet).Path("/api/v1/users/1").Header("Content-Type", "application/json").ResponseString(http.StatusOK, `{"name":"alice"}`)
	ts := r.Server()
	t.Cleanup(func() {
		ts.Close()
	})

	res, err := http.Get(ts.URL + "/api/v1/users/1")
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
	if len(r.Requests()) != 1 {
		t.Errorf("got %v\nwant %v", len(r.Requests()), 1)
	}
}
```

## Response using `examples:` of OpenAPI Document

httpstub can return responses using [`examples:` of OpenAPI Document](https://swagger.io/docs/specification/adding-examples/).

### Use `examples:` in all responses

``` go
ts := httpstub.NewServer(t, httpstub.OpenApi3("path/to/schema.yml"))
t.Cleanup(func() {
	ts.Close()
})
ts.ResponseExample()
```

### Use `examples:` in response to specific endpoint

``` go
ts := httpstub.NewServer(t, httpstub.OpenApi3("path/to/schema.yml"))
t.Cleanup(func() {
	ts.Close()
})
ts.Method(http.MethodGet).Path("/api/v1/users/1").ResponseExample()
```

### Use specific status code `examples:` in the response

It is possible to specify status codes using wildcard.

``` go
ts := httpstub.NewServer(t, httpstub.OpenApi3("path/to/schema.yml"))
t.Cleanup(func() {
	ts.Close()
})
ts.Method(http.MethodPost).Path("/api/v1/users").ResponseExample(httpstub.Status("2*"))
```

## Example

### Stub Twilio

``` go
package client_test

import (
	"net/http"
	"testing"

	"github.com/k1LoW/httpstub"
	twilio "github.com/twilio/twilio-go"
	twclient "github.com/twilio/twilio-go/client"
	api "github.com/twilio/twilio-go/rest/api/v2010"
)

func TestTwilioClient(t *testing.T) {
	r := httpstub.NewRouter(t)
	r.Method(http.MethodPost).Path("/2010-04-01/Accounts/*/Messages.json").ResponseString(http.StatusCreated, `{"status":"sending"}`)
	ts := r.Server()
	t.Cleanup(func() {
		ts.Close()
	})
	tc := ts.Client()

	accountSid := "ACXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
	authToken := "YYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYY"
	client := twilio.NewRestClientWithParams(twilio.ClientParams{
		Client: &twclient.Client{
			Credentials: twclient.NewCredentials(accountSid, authToken),
			HTTPClient:  tc,
		},
	})
	params := &api.CreateMessageParams{}
	params.SetTo("08000000000")
	params.SetFrom("05000000000")
	params.SetBody("Hello there")
	res, err := client.ApiV2010.CreateMessage(params)
	if err != nil {
		t.Error(err)
	}

	got := res.Status
	want := "sending"
	if *got != want {
		t.Errorf("got %v\nwant %v", *got, want)
	}
}
```

## Alternatives

- [github.com/jharlap/httpstub](https://github.com/jharlap/httpstub): Easy stub HTTP servers for testing in Go

