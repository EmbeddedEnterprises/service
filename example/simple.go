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
	"fmt"
	"log/syslog"
	"os"
	"robulab/service"

	"github.com/jcelliott/turnpike"
)

var log *syslog.Writer

func main() {
	srv := service.New(service.Config{
		Name:          "example.simple",
		Serialization: turnpike.MSGPACK,
		Version:       "0.1.0",
		Description:   "Simple example microservice for robµlab.",
	})
	log = srv.Logger
	srv.Connect()

	log.Debug("Trying to register echo procedure in broker...")
	var options = make(map[string]interface{})
	if err := srv.Client.Register("com.robulab.example.echo", echo, options); err != nil {
		log.Emerg(fmt.Sprintf("Failed to register echo procedure in broker: %s", err))
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

	msg := fmt.Sprintf("echo: %s", args...)
	log.Info(msg)
	fmt.Println(msg)

	return nil
}
