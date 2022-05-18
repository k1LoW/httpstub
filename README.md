# httpstub

httpstub provides router ( `http.Handler` ), server ( `*httptest.Server` ) and client ( `*http.Client` ) for stubbing, for testing in Go.

## Usage

``` go
package httpstub

import (
	"io"
	"net/http"
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

or use `NewServer(t *testing.T)` (is syntax sugar)

``` go
package httpstub

import (
	"io"
	"net/http"
	"testing"
)

func TestStub(t *testing.T) {
	ts := NewServer(t)
	t.Cleanup(func() {
		ts.Close()
	})
	ts.Method(http.MethodGet).Path("/api/v1/users/1").Header("Content-Type", "application/json").ResponseString(http.StatusOK, `{"name":"alice"}`)
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
	if len(ts.Requests()) != 1 {
		t.Errorf("got %v\nwant %v", len(ts.Requests()), 1)
	}
}
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
