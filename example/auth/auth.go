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
	"os"

	"github.com/EmbeddedEnterprises/service"
	"github.com/jcelliott/turnpike"
	"github.com/op/go-logging"
)

var log *logging.Logger

func main() {
	srv := service.New(service.Config{
		Name:          "example.auth",
		Serialization: turnpike.JSON,
		Version:       "0.8.0",
		Description:   "Simple example microservice for robµlab using authentication.",
		User:          "WRONG", // set this using $SERVICE_USERNAME
		Password:      "WRONG", // set this using $SERVICE_PASSWORD
		Realm:         "test",
		URL:           "ws://localhost:8000/ws",
	})
	log = srv.Logger
	srv.Connect()

	log.Debug("Trying to register echo procedure in broker...")
	var options = make(map[string]interface{})
	if err := srv.Client.Register("com.robulab.example.echo", echo, options); err != nil {
		log.Criticalf("Failed to register echo procedure in broker: %s", err)
		os.Exit(service.ExitRegistration)
	}
	log.Info("Registered echo procedure")

	srv.Run()
	os.Exit(service.ExitSuccess)
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