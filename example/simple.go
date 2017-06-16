// -*- mode: go; tab-width: 4; -*-

/* simple - robµlab microservice example
 *
 * Copyright (C) 2017  EmbeddedEnterprises
 *     Fin Christensen <christensen.fin@gmail.com>,
 *
 * This file is part of robµlab.
 */

package main

import (
	"os"
	"robulab/service"

	"github.com/jcelliott/turnpike"
	"github.com/op/go-logging"
)

var log *logging.Logger

func main() {
	srv := service.New(service.Config{
		Name:          "example.simple",
		Serialization: turnpike.MSGPACK,
		Version:       "0.3.0",
		Description:   "Simple example microservice for robµlab.",
		Url:           "ws://localhost:8000/ws",
	})
	log = srv.Logger
	srv.Connect()

	log.Debug("Trying to register echo procedure in broker...")
	var options = make(map[string]interface{})
	if err := srv.Client.Register("com.robulab.example.echo", echo, options); err != nil {
		log.Criticalf("Failed to register echo procedure in broker: %s", err)
		os.Exit(service.EXIT_REGISTRATION)
	}
	log.Info("Registered echo procedure")

	srv.Run()
	os.Exit(service.EXIT_SUCCESS)
}

func echo(
	args []interface{},
	kwargs map[string]interface{},
	details map[string]interface{},
) *turnpike.CallResult {
	log.Info("Procedure echo called")
	log.Infof("echo: %s", args...)

	return nil
}
