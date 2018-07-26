package service_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/EmbeddedEnterprises/service"
	"github.com/gammazero/nexus/client"
	"github.com/gammazero/nexus/wamp"
)

func TestIsRPCError(t *testing.T) {
	if service.IsRPCError(errors.New("invalid")) {
		t.Fatal("Expected no RPC error!")
	}

	if !service.IsRPCError(client.RPCError{}) {
		t.Fatal("Expected a RPC error!")
	}

	if service.IsSpecificRPCError(errors.New("invalid"), wamp.ErrNoSuchRealm) {
		t.Fatal("Expected no 'wamp.ErrNoSuchRealm' error!")
	}

	if !service.IsSpecificRPCError(client.RPCError{
		Err: &wamp.Error{
			Error: wamp.ErrNoSuchRealm,
		},
	}, wamp.ErrNoSuchRealm) {
		t.Fatal("Expected a 'wamp.ErrNoSuchRealm' error!")
	}
}

func TestReturnValues(t *testing.T) {
	empty := service.ReturnEmpty()
	if len(empty.Args) > 0 || len(empty.Kwargs) > 0 || empty.Err != "" {
		t.Fatal("Expected empty result!")
	}
	singleValue := service.ReturnValue("returnvalue")
	if len(singleValue.Args) != 1 || singleValue.Args[0] != "returnvalue" {
		t.Fatal("Expected a single argument 'returnvalue'")
	}
	if len(singleValue.Kwargs) > 0 || singleValue.Err != "" {
		t.Fatal("ReturnValue should not return kwargs or error")
	}
	errResult := service.ReturnError("test.error")
	if len(errResult.Args) > 0 || len(errResult.Kwargs) > 0 || errResult.Err != "test.error" {
		t.Fatal("ReturnError should return no kwargs or args")
	}
}

func TestInvalidParse(t *testing.T) {
	invalidDict := wamp.Dict{
		"caller": true,
		"caller_authid": wamp.Dict{
			"invalid": "true",
		},
	}
	caller, err := service.ParseCallerID(invalidDict)
	if err == nil {
		t.Errorf("Expected error during parse, got caller: %v", caller)
		return
	}
}

func TestCallerIDParse(t *testing.T) {
	details := wamp.Dict{
		"caller":        12345,
		"caller_authid": "foo",
		"caller_authrole": wamp.List{
			"trusted",
			"admin",
		},
	}
	caller, err := service.ParseCallerID(details)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if caller.Session != 12345 {
		t.Errorf("Expected session to be '12345', got: %v", caller.Session)
	}
	if caller.Username != "foo" {
		t.Errorf("Expected caller username to be 'foo', got: %v", caller.Session)
	}
	if !caller.HasAnyRole([]string{"trusted"}) {
		t.Error("Expected caller to have role 'trusted'")
	}
	if !caller.HasAnyRole([]string{"admin"}) {
		t.Error("Expected caller to have role 'admin'")
	}
	if caller.HasAnyRole([]string{"foo"}) {
		t.Error("Expected caller to NOT have role 'foo'")
	}
}

func ExampleNew() {
	srv := service.New(service.Config{
		Name:          "example",
		Serialization: client.MSGPACK,
		Version:       "0.1.0",
		Description:   "Simple example microservice from the documentation.",
		URL:           "ws://localhost:8000/ws",
		Timeout:       1 * time.Second,
	})
	srv.Connect()

	// register and subscribe here

	srv.Run()
	os.Exit(service.ExitSuccess)
}

func dummyRegistration(_ context.Context, _ wamp.List, _, _ wamp.Dict) *client.InvokeResult {
	return service.ReturnEmpty()
}

func dummySubscription(_ wamp.List, _, _ wamp.Dict) {
}

func ExampleService_RegisterAll() {
	srv := service.New(service.Config{
		Name:          "example",
		Serialization: client.MSGPACK,
		Version:       "0.1.0",
		Description:   "Simple example microservice from the documentation.",
		URL:           "ws://localhost:8000/ws",
	})
	srv.Connect()

	options := wamp.Dict{}
	procedures := map[string]service.HandlerRegistration{
		"example.get_magic":     {Handler: dummyRegistration, Options: options},
		"example.do_stuff":      {Handler: dummyRegistration, Options: options},
		"example.set_something": {Handler: dummyRegistration, Options: options},
	}
	if err := srv.RegisterAll(procedures); err != nil {
		srv.Logger.Criticalf(
			"Failed to register procedure '%s' in broker: %s",
			err.ProcedureName,
			err,
		)
		os.Exit(service.ExitRegistration)
	}

	srv.Run()
	os.Exit(service.ExitSuccess)
}

func ExampleService_SubscribeAll() {
	srv := service.New(service.Config{
		Name:          "example",
		Serialization: client.MSGPACK,
		Version:       "0.1.0",
		Description:   "Simple example microservice from the documentation.",
		URL:           "ws://localhost:8000/ws",
	})
	srv.Connect()

	options := wamp.Dict{}
	events := map[string]service.EventSubscription{
		"example.goo_happened": {Handler: dummySubscription, Options: options},
		"example.gesus_joined": {Handler: dummySubscription, Options: options},
		"example.no_more_mate": {Handler: dummySubscription, Options: options},
	}
	if err := srv.SubscribeAll(events); err != nil {
		srv.Logger.Criticalf(
			"Failed to subscribe to topic '%s' in broker: %s",
			err.Topic,
			err,
		)
		os.Exit(service.ExitRegistration)
	}

	srv.Run()
	os.Exit(service.ExitSuccess)
}
