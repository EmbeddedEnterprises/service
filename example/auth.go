// -*- mode: go; tab-width: 4; -*-

/* simple - robµlab microservice example
 *
 * Copyright (C) 2017  EmbeddedEnterprises
 *     Fin Christensen <christensen.fin@gmail.com>,
 *     Martin Koppehel <martin.koppehel@s.ovgu.de>,
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
		Name:          "example.auth",
		Serialization: turnpike.JSON,
		Version:       "0.2.0",
		Description:   "Simple example microservice for robµlab using authentication.",
		User:          "WRONG", // set this using $SERVICE_USERNAME
		Password:      "WRONG", // set this using $SERVICE_PASSWORD
		Realm:         "test",
		Url:           "ws://localhost:8000/ws",
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
