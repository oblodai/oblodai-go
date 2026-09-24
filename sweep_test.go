package oblodai_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/oblodai/oblodai-go/v2"
	"github.com/oblodai/oblodai-go/v2/internal/fakeapi"
)

// Every operation of the contract is reachable through exactly one generated method, and that
// method sends the operation's own request line: each method of each service is called with
// placeholder arguments against a fake gateway that names the route it received.
func TestEveryOperationHasOneWorkingMethod(t *testing.T) {
	api := fakeapi.New(t, nil)
	client := api.Client(t, oblodai.WithAdminToken("adm"))
	ctx := context.Background()
	called := map[string]string{}

	services := reflect.ValueOf(client.Resources)
	for i := 0; i < services.NumField(); i++ {
		service := services.Field(i)
		for m := 0; m < service.NumMethod(); m++ {
			method := service.Method(m)
			name := services.Type().Field(i).Name + "." + service.Type().Method(m).Name
			before := len(api.Requests())
			out := method.Call(placeholderArgs(ctx, method.Type()))
			if list := out[0]; len(out) == 1 {
				// A paged list requests nothing until it is read.
				page := list.MethodByName("Page").Call(nil)
				if err := page[1].Interface(); err != nil {
					t.Errorf("%s: %v", name, err)
					continue
				}
			} else if err := out[1].Interface(); err != nil {
				t.Errorf("%s: %v", name, err)
				continue
			}
			requests := api.Requests()[before:]
			if len(requests) != 1 {
				t.Errorf("%s made %d requests", name, len(requests))
				continue
			}
			op := requests[0].OperationID
			if prev, dup := called[op]; dup {
				t.Errorf("%s and %s both call %s", prev, name, op)
			}
			called[op] = name
		}
	}
	for op := range oblodai.Routes {
		if _, ok := called[op]; !ok {
			t.Errorf("no method calls %s", op)
		}
	}
	if len(called) != len(oblodai.Routes) {
		t.Errorf("%d methods for %d operations", len(called), len(oblodai.Routes))
	}
}

// placeholderArgs: the context, "x-<n>" for each path parameter, and a zero value — nil params —
// for the rest.
func placeholderArgs(ctx context.Context, fn reflect.Type) []reflect.Value {
	args := []reflect.Value{reflect.ValueOf(ctx)}
	for i := 1; i < fn.NumIn(); i++ {
		in := fn.In(i)
		switch {
		case fn.IsVariadic() && i == fn.NumIn()-1:
		case in.Kind() == reflect.String:
			args = append(args, reflect.ValueOf("x-"+strings.Repeat("1", i)))
		case in.Kind() == reflect.Pointer && in.Elem().Kind() == reflect.Struct:
			// Required query parameters must be set for the request to be meaningful, but any
			// value will do for routing; a zero struct sends zero values.
			args = append(args, reflect.New(in.Elem()))
		default:
			args = append(args, reflect.Zero(in))
		}
	}
	return args
}
