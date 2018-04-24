/* service - robµlab convenience wrapper for easy microservice creation.
 *
 * Copyright (C) 2017-2018  EmbeddedEnterprises
 *     Fin Christensen <christensen.fin@gmail.com>,
 *     Martin Koppehel <martin.koppehel@st.ovgu.de>,
 *
 * This file is part of robµlab.
 */

package service

import (
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
const BROKER_URL_ENV string = "SERVICE_BROKER_URL"

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
		fmt.Fprintf(os.Stderr, "\n%s copyright © 2017-2018  EmbeddedEnterprises\n", default_config.Name)
	}

	//default values for service name, serialization, websocket URL and realm
	name := default_config.Name
	if name == "" {
		name = "example"
	}
	serialization := default_config.Serialization
	url := default_config.Url
	env_url := os.Getenv(BROKER_URL_ENV)
	if env_url != "" {
		url = env_url
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

	if *cli_url == "" {
		fmt.Printf("Please provide a broker url!\n")
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
	self.Client, err = turnpike.NewWebsocketClient(self.serialization, self.url, nil, nil, nil)
	if err != nil {
		self.Logger.Criticalf("Failed to connect service to broker: %s", err)
		os.Exit(EXIT_CONNECT)
	}

	if self.use_auth {
		auth_methods := make(map[string]turnpike.AuthFunc)
		auth_methods["ticket"] = func(_, _ map[string]interface{}) (string, map[string]interface{}, error) {
			return self.password, make(map[string]interface{}), nil
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

func (self *Service) Run() {
	var err error

	sigint_channel := make(chan os.Signal, 1)
	signal.Notify(sigint_channel, os.Interrupt)
	close_channel := make(chan bool, 1)

	self.Logger.Info("Entering main loop")
	fmt.Println("Send SIGINT to quit")
	self.Client.ReceiveDone = close_channel
	select {
	case <-sigint_channel:
		// linebreak after echoed ^C
		fmt.Println()
		self.Logger.Info("Received SIGINT, exiting")
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

	case <-close_channel:
		self.Logger.Info("Connection lost, exiting")
	}
	self.Logger.Info("Leaving main loop")
	self.Logger.Info("Bye")
}

type RegistrationError struct {
	ProcedureName string
	Inner         error
}

type SubscribtionError struct {
	Topic string
	Inner error
}

type HandlerRegistration struct {
	Handler turnpike.MethodHandler
	Options map[string]interface{}
}

type EventSubscribtion struct {
	Handler turnpike.EventHandler
	Options map[string]interface{}
}

func (self *Service) RegisterAll(procedures map[string]HandlerRegistration) *RegistrationError {
	for name, regr := range procedures {
		if err := self.Client.Register(name, regr.Handler, regr.Options); err != nil {
			return &RegistrationError{
				ProcedureName: name,
				Inner:         err,
			}
		}
	}

	return nil
}

func (self *Service) SubscribeAll(procedures map[string]EventSubscribtion) *SubscribtionError {
	for topic, regr := range procedures {
		if err := self.Client.Subscribe(topic, regr.Options, regr.Handler); err != nil {
			return &SubscribtionError{
				Topic: topic,
				Inner: err,
			}
		}
	}

	return nil
}

type ErrorKind int

const (
	ErrorBadArgument ErrorKind = iota
	ErrorNotAvailable
	ErrorNotEnoughData
	ErrorUnexpectedData
	ErrorTooMuchData
	ErrorOutOfRange
	ErrorTimedOut
	ErrorPermissionDenied
	ErrorNotFound
	ErrorUnreachableLineReached
	ErrorThisWorksOnMyMachine
	ErrorItsNotABugItsAFeature
	ErrorAKittenDies
)

type Error struct {
	kind  ErrorKind
	inner error
}

func NewError(kind ErrorKind) *Error {
	return &Error{
		kind:  kind,
		inner: nil,
	}
}

func NewErrorFrom(kind ErrorKind, inner error) *Error {
	return &Error{
		kind:  kind,
		inner: inner,
	}
}

func (e *Error) Error() string {
	var msg string

	switch e.kind {
	case ErrorBadArgument:
		msg = "A given argument does not meet its requirements."
	case ErrorNotAvailable:
		msg = "A requested resource is not available."
	case ErrorNotEnoughData:
		msg = "The provided data is not enough."
	case ErrorUnexpectedData:
		msg = "The provided data is in an unexpected format."
	case ErrorTooMuchData:
		msg = "The provided data is too much."
	case ErrorOutOfRange:
		msg = "A given index is out of range."
	case ErrorTimedOut:
		msg = "A request has timed out."
	case ErrorPermissionDenied:
		msg = "The permission to a resource got denied."
	case ErrorNotFound:
		msg = "A given resource could not be found."
	case ErrorUnreachableLineReached:
		msg = "This code should not be reached as it is not implemented."
	case ErrorThisWorksOnMyMachine:
		msg = "Code that needs complicated state to work. Contact your system administrator for details."
	case ErrorItsNotABugItsAFeature:
		msg = "The current behavior is intended. If you did not expect this to happen, contact your system administrator."
	case ErrorAKittenDies:
		msg = "Something was nil..."
	default:
		msg = "Unknown error occured."
	}

	if e.inner != nil {
		return fmt.Sprintf("%s\nInner Error: %s", msg, e.inner.Error())
	}
	return msg
}
