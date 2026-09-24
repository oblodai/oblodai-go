package oblodai

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// An error reads in a log the way support needs it: [code] message (request_id=…).
func TestErrorStringIsCodeMessageAndRequestID(t *testing.T) {
	cases := []struct {
		err  *Error
		want string
	}{
		{&Error{Code: "payout.insufficient_funds", Message: "not enough USDT", RequestID: "rq-1", HTTPStatus: 400},
			"[payout.insufficient_funds] not enough USDT (request_id=rq-1)"},
		{&Error{Code: "sdk.bad_config", Message: "the base URL is not a valid URL"},
			"[sdk.bad_config] the base URL is not a valid URL"},
		{&Error{Code: "internal"}, "[internal]"},
	}
	for _, c := range cases {
		if got := c.err.Error(); got != c.want {
			t.Errorf("got  %q\nwant %q", got, c.want)
		}
		if got := fmt.Sprint(c.err); got != c.want {
			t.Errorf("fmt: %q", got)
		}
	}
}

func TestAnAPIErrorPrintsTheCallsRequestID(t *testing.T) {
	api := newFakeAPI(t, apiError(404, map[string]any{"code": "payment.not_found", "message": "no such payment", "retryable": false}))
	_, err := api.client().Payments.GetInfo(context.Background(), &LookupRequest{UUID: Ptr("u")}, WithRequestID("rid-9"))
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatal(err)
	}
	if got := err.Error(); got != "[payment.not_found] no such payment (request_id=rid-9)" {
		t.Fatalf("got %q", got)
	}
}
