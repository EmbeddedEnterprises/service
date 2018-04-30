/* simple - robµlab microservice example
 *
 * Copyright (C) 2017  EmbeddedEnterprises
 *     Fin Christensen <christensen.fin@gmail.com>,
 *
 * This file is part of robµlab.
 */

package main

import (
	"context"
	"os"

	"github.com/EmbeddedEnterprises/service"
	"github.com/gammazero/nexus/client"
	"github.com/gammazero/nexus/wamp"
	"github.com/op/go-logging"
)

var log *logging.Logger

func main() {
	srv := service.New(service.Config{
		Name:          "example.simple",
		Serialization: client.MSGPACK,
		Version:       "0.1.1",
		Description:   "Simple example microservice for robµlab.",
		URL:           "ws://localhost:8000/ws",
		Realm:         "realm1",
	})
	log = srv.Logger
	srv.Connect()

	log.Debug("Trying to register echo procedure in broker...")
	var options = wamp.Dict{}
	if err := srv.Client.Register("com.robulab.example.echo", echo, options); err != nil {
		log.Criticalf("Failed to register echo procedure in broker: %s", err)
		os.Exit(service.ExitRegistration)
	}
	log.Info("Registered echo procedure")

	srv.Run()
	os.Exit(service.ExitSuccess)
}

func echo(_ context.Context, args wamp.List, kwargs, details wamp.Dict) *client.InvokeResult {
	log.Info("Procedure echo called")
	log.Infof("args: %v", args...)
	log.Infof("kwargs: %v", kwargs)
	log.Infof("details: %v", details)

	return service.ReturnEmpty()
}
