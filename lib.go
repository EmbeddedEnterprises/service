// -*- mode: go; tab-width: 4; -*-

/* service - robµlab convenience wrapper for easy microservice creation.
 *
 * Copyright (C) 2017  EmbeddedEnterprises
 *     Fin Christensen <christensen.fin@gmail.com>,
 *
 * This file is part of robµlab.
 */

package service

import (
	"fmt"
	"log/syslog"
	"os"
	"os/signal"

	"github.com/jcelliott/turnpike"
	flag "github.com/ogier/pflag"
)

const (
	EXIT_SUCCESS int = iota
	EXIT_ARGUMENT
	EXIT_SERVICE
	EXIT_CONNECT
	EXIT_REGISTRATION
)

type Service struct {
	name          string
	serialization turnpike.Serialization
	realm         string
	url           string
	Logger        *syslog.Writer
	Client        *turnpike.Client
}

type Config struct {
	Name          string
	Version       string
	Description   string
	LogPriority   syslog.Priority
	Serialization turnpike.Serialization
	Url           string
	Realm         string
}

func New(default_config Config) *Service {
	var err error

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTION]...\n\n%s\n\nOptions:\n", os.Args[0], default_config.Description)
		fmt.Fprintln(os.Stderr, "  -h, --help\n    \tprint this help message")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\n%s copyright © 2017  EmbeddedEnterprises\n", default_config.Name)
	}

	name := default_config.Name
	if name == "" {
		name = "example"
	}

	log_priority := default_config.LogPriority
	serialization := default_config.Serialization

	url := default_config.Url
	if url == "" {
		url = "ws://localhost:8000/ws"
	}

	realm := default_config.Realm
	if realm == "" {
		realm = "realm1"
	}

	srv := &Service{}
	srv.name = name

	srv.Logger, err = syslog.New(log_priority, "com.robulab."+name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating service: %s\n", err)
		os.Exit(EXIT_SERVICE)
	}

	srv.Logger.Info("Hello")

	var def_ser string
	if serialization == turnpike.JSON {
		def_ser = "json"
	} else if serialization == turnpike.MSGPACK {
		def_ser = "msgpack"
	}
	var cli_ver = flag.BoolP("version", "V", false, "prints the version")
	var cli_ser = flag.StringP("serialization", "s", def_ser, "the value may be one of json or msgpack")
	var cli_url = flag.StringP("url", "u", url, "the websocket url of the broker")
	var cli_rlm = flag.StringP("realm", "r", realm, "the name of the realm to connect to")

	flag.Parse()

	if *cli_ver {
		fmt.Println(default_config.Version)
		os.Exit(EXIT_SUCCESS)
	}

	if *cli_ser == "json" {
		srv.serialization = turnpike.JSON
	} else if *cli_ser == "msgpack" {
		srv.serialization = turnpike.MSGPACK
	} else {
		fmt.Printf("Invalid serialization type '%s'!\n", *cli_ser)
		flag.Usage()
		os.Exit(EXIT_ARGUMENT)
	}

	srv.url = *cli_url
	srv.realm = *cli_rlm

	info_url := fmt.Sprintf("Using '%s' as connection url...", srv.url)
	info_ser := fmt.Sprintf("Using '%s' as serialization type...", *cli_ser)
	info_rlm := fmt.Sprintf("Using '%s' as realm...", srv.realm)
	srv.Logger.Info(info_url)
	srv.Logger.Info(info_ser)
	srv.Logger.Info(info_rlm)

	return srv
}

func (self *Service) Connect() {
	var err error

	self.Logger.Debug("Trying to connect to broker")
	self.Client, err = turnpike.NewWebsocketClient(self.serialization, self.url, nil, nil)
	if err != nil {
		s := fmt.Sprintf("Failed to connect service to broker: %s", err)
		self.Logger.Emerg(s)
		os.Exit(EXIT_CONNECT)
	}

	self.Logger.Info("Connected to broker")

	self.Logger.Debug("Trying to join realm...")
	_, err = self.Client.JoinRealm(self.realm, nil)
	if err != nil {
		s := fmt.Sprintf("Failed to connect service to broker: %s", err)
		self.Logger.Emerg(s)
		os.Exit(EXIT_CONNECT)
	}

	self.Logger.Info("Joined realm")
}

func (self Service) Run() {
	var err error

	sigint_channel := make(chan os.Signal, 1)
	signal.Notify(sigint_channel, os.Interrupt)

	self.Logger.Info("Entering main loop")
	fmt.Println("Press Ctrl-C to quit")
	<-sigint_channel
	self.Logger.Info("Leaving main loop")

	// linebreak after echoed ^C
	fmt.Println()

	err = self.Client.LeaveRealm()
	if err != nil {
		msg := fmt.Sprintf("Error while running the service: %s", err)
		self.Logger.Emerg(msg)
		os.Exit(EXIT_SERVICE)
	}

	err = self.Client.Close()
	if err != nil {
		msg := fmt.Sprintf("Error while running the service: %s", err)
		self.Logger.Emerg(msg)
		os.Exit(EXIT_SERVICE)
	}

	self.Logger.Info("Bye")

	err = self.Logger.Close()
	if err != nil {
		msg := fmt.Sprintf("Error while running the service: %s", err)
		fmt.Fprintln(os.Stderr, msg)
		os.Exit(EXIT_SERVICE)
	}
}
