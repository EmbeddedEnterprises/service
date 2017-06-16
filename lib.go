// -*- mode: go; tab-width: 4; -*-

/* service - robµlab convenience wrapper for easy microservice creation.
 *
 * Copyright (C) 2017  EmbeddedEnterprises
 *     Fin Christensen <christensen.fin@gmail.com>,
 *     Martin Koppehel <martin.koppehel@st.ovgu.de>,
 *
 * This file is part of robµlab.
 */

package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/jcelliott/turnpike"
	flag "github.com/ogier/pflag"
	"github.com/op/go-logging"
)

const (
	EXIT_SUCCESS int = iota
	EXIT_ARGUMENT
	EXIT_SERVICE
	EXIT_CONNECT
	EXIT_REGISTRATION
)

const USERNAME_ENV string = "SERVICE_USERNAME"
const PASSWORD_ENV string = "SERVICE_PASSWORD"
const LOG_FORMAT_ENV string = "SERVICE_LOGFORMAT"

type Service struct {
	name          string
	serialization turnpike.Serialization
	realm         string
	url           string
	username      string
	password      string
	use_auth      bool
	Logger        *logging.Logger
	Client        *turnpike.Client
}

type Config struct {
	Name          string
	Version       string
	Description   string
	Serialization turnpike.Serialization
	Url           string
	Realm         string
	User          string
	Password      string
}

func New(default_config Config) *Service {
	var err error

	// additional usage information
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTION]...\n\n%s\n\nOptions:\n", os.Args[0], default_config.Description)
		fmt.Fprintln(os.Stderr, "  -h, --help\n    \tprint this help message")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\n%s copyright © 2017  EmbeddedEnterprises\n", default_config.Name)
	}

	//default values for service name, serialization, websocket URL and realm
	name := default_config.Name
	if name == "" {
		name = "example"
	}
	serialization := default_config.Serialization
	url := default_config.Url
	if url == "" {
		url = "ws://communication.robulab.svc.cluster.local:8000/ws"
	}
	realm := default_config.Realm
	if realm == "" {
		realm = "robulab"
	}

	// create a new service object on the heap
	srv := &Service{}
	srv.name = name

	// translate serialization enums to strings to allow CLI parsing
	var def_ser string
	if serialization == turnpike.JSON {
		def_ser = "json"
	} else if serialization == turnpike.MSGPACK {
		def_ser = "msgpack"
	}

	// fetch username and password from environment variables overwriting
	// the default values
	user := default_config.User
	env_user := os.Getenv(USERNAME_ENV)
	if env_user != "" {
		user = env_user
	}
	password := default_config.Password
	env_pass := os.Getenv(PASSWORD_ENV)
	if env_pass != "" {
		password = env_pass
	}

	// build the command line interface, allow to override many default values
	var cli_ver = flag.BoolP("version", "V", false, "prints the version")
	var cli_ser = flag.StringP("serialization", "s", def_ser, "the value may be one of json or msgpack")
	var cli_url = flag.StringP("broker-url", "b", url, "the websocket url of the broker")
	var cli_usr = flag.StringP("user", "u", user, "the user to login as")
	var cli_pwd = flag.StringP("password", "p", password, "the password to login with")
	var cli_rlm = flag.StringP("realm", "r", realm, "the name of the realm to connect to")

	// parse the command line
	flag.Parse()

	// display version information
	if *cli_ver {
		fmt.Println(default_config.Version)
		os.Exit(EXIT_SUCCESS)
	}

	// translate the serialization from the CLI to turnpike enum
	if *cli_ser == "json" {
		srv.serialization = turnpike.JSON
	} else if *cli_ser == "msgpack" {
		srv.serialization = turnpike.MSGPACK
	} else {
		fmt.Printf("Invalid serialization type '%s'!\n", *cli_ser)
		flag.Usage()
		os.Exit(EXIT_ARGUMENT)
	}

	// setup the final values to use for this service
	srv.url = *cli_url
	srv.realm = *cli_rlm
	srv.use_auth = true
	if *cli_usr == "" || *cli_pwd == "" {
		srv.use_auth = false
	}
	srv.username = *cli_usr
	srv.password = *cli_pwd

	// setup logging library

	srv.Logger, err = logging.GetLogger("com.robulab." + name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating logger: %s\n", err)
		os.Exit(EXIT_SERVICE)
	}

	// write to Stderr to keep Stdout free for data output
	backend := logging.NewLogBackend(os.Stderr, "", 0)

	// read an environment variable controlling the log format
	// possibilities are "k8s" or "cluster" or "machine" for a machine readable format
	// and "debug" or "human" for a human readable format (default)
	// the values are case insensitive
	var log_format logging.Formatter
	env_log_format := strings.ToLower(os.Getenv(LOG_FORMAT_ENV))
	switch env_log_format {
	case "", "human", "debug":
		log_format, err = logging.NewStringFormatter(`%{color}[%{level:-8s}] %{time:15:04:05.000} %{longpkg}@%{shortfile}%{color:reset} -- %{message}`)
	case "k8s", "cluster", "machine":
		log_format, err = logging.NewStringFormatter(`[%{level:-8s}] %{time:2006-01-02T15:04:05.000} %{shortfunc} -- %{message}`)
	default:
		fmt.Fprintf(os.Stderr, "Failed to setup log format: invalid format %s", env_log_format)
		os.Exit(EXIT_SERVICE)
	}
	if err != nil {
		srv.Logger.Criticalf("Failed to create logging format, shutting down: %s", err)
		os.Exit(EXIT_SERVICE)
	}

	backend_formatted := logging.NewBackendFormatter(backend, log_format)
	logging.SetBackend(backend_formatted)

	srv.Logger.Info("Hello")
	srv.Logger.Infof("Using '%s' as connection url...", srv.url)
	srv.Logger.Infof("Using '%s' as serialization type...", *cli_ser)
	srv.Logger.Infof("Using '%s' as realm...", srv.realm)
	srv.Logger.Infof("Using '%s' as user-id...", srv.username)

	return srv
}

func (self *Service) Connect() {
	var err error

	self.Logger.Debug("Trying to connect to broker")
	self.Client, err = turnpike.NewWebsocketClient(self.serialization, self.url, nil, nil)
	if err != nil {
		self.Logger.Criticalf("Failed to connect service to broker: %s", err)
		os.Exit(EXIT_CONNECT)
	}

	if self.use_auth {
		auth_methods := make(map[string]turnpike.AuthFunc)
		auth_methods["wampcra"] = func(h, c map[string]interface{}) (string, map[string]interface{}, error) {
			// use a standard WAMP-CRA authentication here
			// we use the password as key for the HMAC SHA256.

			challenge, ok := c["challenge"].(string)
			extra := make(map[string]interface{})

			if !ok {
				self.Logger.Warning("no challenge data received")
				return "", extra, errors.New("no challenge data received")
			}

			self.Logger.Debugf("Got challenge: %s", challenge)

			mac := hmac.New(sha256.New, []byte(self.password))
			mac.Write([]byte(challenge))
			signature := mac.Sum(nil)

			return base64.StdEncoding.EncodeToString(signature), extra, nil
		}

		self.Client.Auth = auth_methods
	}
	self.Logger.Info("Connected to broker")

	self.Logger.Debug("Trying to join realm...")

	var join_realm_details map[string]interface{}
	if self.use_auth {
		join_realm_details = make(map[string]interface{})
		join_realm_details["authid"] = self.username
		self.Logger.Debugf("Login: %s", join_realm_details["authid"])
	}
	_, err = self.Client.JoinRealm(self.realm, join_realm_details)
	if err != nil {
		self.Logger.Criticalf("Failed to join realm: %s", err)
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
		self.Logger.Criticalf("Error while running the service: %s", err)
		os.Exit(EXIT_SERVICE)
	}

	err = self.Client.Close()
	if err != nil {
		self.Logger.Criticalf("Error while running the service: %s", err)
		os.Exit(EXIT_SERVICE)
	}

	self.Logger.Info("Bye")
}
