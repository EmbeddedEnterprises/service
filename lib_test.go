package service_test

import (
	"os"
	"context"

	"github.com/EmbeddedEnterprises/service"
	"github.com/gammazero/nexus/client"
	"github.com/gammazero/nexus/wamp"
)

func ExampleNew() {
	srv := service.New(service.Config{
		Name:          "example",
		Serialization: client.MSGPACK,
		Version:       "0.1.0",
		Description:   "Simple example microservice from the documentation.",
		URL:           "ws://localhost:8000/ws",
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
		"example.get_magic":      service.HandlerRegistration{dummyRegistration, options},
		"example.do_stuff":       service.HandlerRegistration{dummyRegistration, options},
		"example.set_something":  service.HandlerRegistration{dummyRegistration, options},
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
		"example.goo_happened":   service.EventSubscription{dummySubscription, options},
		"example.gesus_joined":   service.EventSubscription{dummySubscription, options},
		"example.no_more_mate":   service.EventSubscription{dummySubscription, options},
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
