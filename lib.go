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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/gammazero/nexus/client"
	"github.com/gammazero/nexus/transport/serialize"
	"github.com/gammazero/nexus/wamp"
	flag "github.com/ogier/pflag"
	"github.com/op/go-logging"
)

const (
	// ExitSuccess indicates that the service terminated without an error.
	ExitSuccess int = iota

	// ExitArgument indicates that the service terminated early as an argument was missing or malformed.
	ExitArgument

	// ExitService indicates that the service implementation ran into an unhandled error and could not be recovered.
	ExitService

	// ExitConnect indicates that the service failed to connect to the broker.
	ExitConnect

	// ExitRegistration indicates that the service failed to register or subscribe for a given topic or method.
	ExitRegistration
)

// EnvUsername defines the environment variable name for the username the service is using
// to authenticate on the broker.
const EnvUsername string = "SERVICE_USERNAME"

// EnvPassword defines the environment variable name for the password the service is using
// to authenticate on the broker.
const EnvPassword string = "SERVICE_PASSWORD"

// EnvLogFormat defines the environment variable name for the logging format string definition.
const EnvLogFormat string = "SERVICE_LOGFORMAT"

// EnvBrokerURL defines the environment variable name for the broker url definition.
const EnvBrokerURL string = "SERVICE_BROKER_URL"

// EnvRealm defines the environment variable name for the realm definition.
const EnvRealm string = "SERVICE_REALM"

// EnvSerialization defines the environment variable name for the serialization definition.
// Possible values are json and msgpack
const EnvSerialization string = "SERVICE_SERIALIZATION"

// EnvTlsClientCertFile defines the environment variable name for the TLS client certificate
// public key to present to the router.
const EnvTlsClientCertFile string = "TLS_CLIENT_CERT"

// EnvTlsClientKeyFile defines the environment variable name for the TLS client certificate
// private key to present to the router.
const EnvTlsClientKeyFile string = "TLS_CLIENT_KEY"

// EnvTlsServerCertFile defines the environment variable name for the TLS server certificate
// public key to verify the server certificate against.
const EnvTlsServerCertFile string = "TLS_SERVER_CERT"

// Version defines the git tag this code is built with
const Version string = "0.11.0"

// Service is a struct that holds all state that is needed to run the service.
// An instance of this struct is the main object that is used to communicate with the
// broker backend. Use the `New` function to create a service instance. The instance will
// give you access to the `Logger` and `Client` object.
type Service struct {
	name          string
	serialization serialize.Serialization
	realm         string
	url           string
	username      string
	password      string
	useAuth       bool
	useTls        bool
	serverCert    *x509.CertPool
	clientCert    *tls.Certificate
	Logger        *logging.Logger
	Client        *client.Client
}

// Config holds the default configuration that is applied to a `Service` instance when the configuration
// is not overridden by a cli argument or an environment variable. The cli argument always has priority!
type Config struct {
	Name              string
	Version           string
	Description       string
	Serialization     serialize.Serialization
	URL               string
	Realm             string
	User              string
	Password          string
	TLSClientCertFile string
	TLSClientKeyFile  string
	TLSServerCertFile string
}

// This function allows to query environment variables with default values.
func overwriteEnv(defaultValue string, envVar string) string {
	envValue := os.Getenv(envVar)
	if envValue != "" {
		return envValue
	}
	return defaultValue
}

func ensureFileExists(fid, fname string, srv *Service) {
	if _, err := os.Stat(fname); os.IsNotExist(err) {
		srv.Logger.Errorf("Error validating %s: file %s doesn't exist! Exiting.\n", fid, fname)
		os.Exit(ExitArgument)
	}
}

func setupLogger(srv *Service) {
	// setup logging library
	var err error
	srv.Logger, err = logging.GetLogger("com.robulab." + srv.name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating logger: %s\n", err)
		os.Exit(ExitService)
	}

	// write to Stderr to keep Stdout free for data output
	backend := logging.NewLogBackend(os.Stderr, "", 0)

	// read an environment variable controlling the log format
	// possibilities are "k8s" or "cluster" or "machine" for a machine readable format
	// and "debug" or "human" for a human readable format (default)
	// the values are case insensitive
	var logFormat logging.Formatter
	envLogFormat := strings.ToLower(os.Getenv(EnvLogFormat))
	switch envLogFormat {
	case "", "human", "debug":
		logFormat, err = logging.NewStringFormatter(`%{color}[%{level:-8s}] %{time:15:04:05.000} %{longpkg}@%{shortfile}%{color:reset} -- %{message}`)
	case "k8s", "cluster", "machine":
		logFormat, err = logging.NewStringFormatter(`[%{level:-8s}] %{time:2006-01-02T15:04:05.000} %{shortfunc} -- %{message}`)
	default:
		fmt.Fprintf(os.Stderr, "Failed to setup log format: invalid format %s", envLogFormat)
		os.Exit(ExitArgument)
	}
	if err != nil {
		srv.Logger.Criticalf("Failed to create logging format, shutting down: %s", err)
		os.Exit(ExitArgument)
	}

	backendFormatted := logging.NewBackendFormatter(backend, logFormat)
	logging.SetBackend(backendFormatted)
}

// New creates a new service instance from the provided default configuration.
// The configuration can be overridden with command line arguments or environment variables.
// The main function of your microservice will most likely look like this:
//
// 	func main() {
// 		srv := service.New(service.Config{
// 			Name:          "example",
// 			Serialization: client.MSGPACK,
// 			Version:       "0.1.0",
// 			Description:   "Simple example microservice from the documentation.",
// 			URL:           "ws://localhost:8000/ws",
// 		})
// 		srv.Connect()
//
// 		// register and subscribe here
//
// 		srv.Run()
// 		os.Exit(service.ExitSuccess)
// 	}
//
// You can look in the `examples` of the source repository for a more detailed example.
//
// This function can exit the program early when
//
// 1. A version print was requested by the command line interface.
//
// 2. An error occurred while parsing the command line arguments.
//
// 3. An internal error occurrs that cannot be recovered.
func New(defaultConfig Config) *Service {
	// additional usage information
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTION]...\n\n%s\n\nOptions:\n", os.Args[0], defaultConfig.Description)
		fmt.Fprintln(os.Stderr, "  -h, --help\n    \tprint this help message")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\n%s copyright © 2017-2018  EmbeddedEnterprises\n", defaultConfig.Name)
	}

	//default values for service name, serialization, websocket URL and realm
	name := defaultConfig.Name
	if name == "" {
		name = "example"
	}

	serialization := defaultConfig.Serialization
	// translate serialization enums to strings to allow CLI parsing
	var defSer string
	if serialization == client.JSON {
		defSer = "json"
	} else if serialization == client.MSGPACK {
		defSer = "msgpack"
	}

	// fetch username and password from environment variables overwriting
	// the default values
	url := overwriteEnv(defaultConfig.URL, EnvBrokerURL)
	realm := overwriteEnv(defaultConfig.Realm, EnvRealm)
	defSer = overwriteEnv(defSer, EnvSerialization)
	user := overwriteEnv(defaultConfig.User, EnvUsername)
	password := overwriteEnv(defaultConfig.Password, EnvPassword)
	clientCertFile := overwriteEnv(defaultConfig.TLSClientCertFile, EnvTlsClientCertFile)
	clientKeyFile := overwriteEnv(defaultConfig.TLSClientKeyFile, EnvTlsClientKeyFile)
	serverCertFile := overwriteEnv(defaultConfig.TLSServerCertFile, EnvTlsServerCertFile)

	// build the command line interface, allow to override many default values
	var cliVer = flag.BoolP("version", "V", false, "prints the version")
	var cliSer = flag.StringP("serialization", "s", defSer, "the value may be one of json or msgpack")
	var cliURL = flag.StringP("broker-url", "b", url, "the websocket url of the broker")
	var cliUsr = flag.StringP("user", "u", user, "the user to login as")
	var cliPwd = flag.StringP("password", "p", password, "the password to login with")
	var cliRlm = flag.StringP("realm", "r", realm, "the name of the realm to connect to")
	var cliCCF = flag.String("tls-client-cert-file", clientCertFile, "TLS client public key file")
	var cliCKF = flag.String("tls-client-key-file", clientKeyFile, "TLS client private key file")
	var cliSCF = flag.String("tls-server-cert-file", serverCertFile, "TLS server public key file")

	// parse the command line
	flag.Parse()

	// display version information
	if *cliVer {
		fmt.Printf("Version (service-lib): %s\n", Version)
		fmt.Printf("Version (%s): %s\n", defaultConfig.Name, defaultConfig.Version)
		os.Exit(ExitSuccess)
	}

	// create a new service object on the heap
	srv := &Service{}
	srv.name = name
	setupLogger(srv)

	// translate the serialization from the CLI to nexus enum
	if *cliSer == "json" {
		srv.serialization = client.JSON
	} else if *cliSer == "msgpack" {
		srv.serialization = client.MSGPACK
	} else {
		srv.Logger.Errorf("Invalid serialization type '%s'!", *cliSer)
		flag.Usage()
		os.Exit(ExitArgument)
	}

	if *cliURL == "" {
		srv.Logger.Error("Please provide a broker url!")
		flag.Usage()
		os.Exit(ExitArgument)
	}

	// setup the final values to use for this service
	srv.url = *cliURL
	srv.realm = *cliRlm
	srv.useAuth = true

	// when wss:// is set, we are using TLS to secure the connection.
	if strings.HasPrefix(srv.url, "wss://") {
		srv.useTls = true
		if *cliSCF == "" {
			srv.Logger.Warning("Server Certificate/CA not set, disabling verification!")
			srv.serverCert = nil
		} else {
			ensureFileExists("TLS server public key", *cliSCF, srv)
			srv.serverCert = x509.NewCertPool()
			certPEM, err := ioutil.ReadFile(*cliSCF)
			if err != nil {
				srv.Logger.Errorf("Failed to load TLS server public key: %s", err)
				os.Exit(ExitArgument)
			}
			if !srv.serverCert.AppendCertsFromPEM(certPEM) {
				srv.Logger.Error("Failed to import server certificate/CA to trust!")
				os.Exit(ExitArgument)
			}
		}

		if *cliCCF == "" || *cliCKF == "" {
			srv.Logger.Info("TLS client certificate not provided, falling back to ticket auth")
			srv.clientCert = nil
			if *cliUsr == "" || *cliPwd == "" {
				srv.Logger.Warning("Missing username/password, disabling authentication completely.")
				srv.useAuth = false
			}
			srv.username = *cliUsr
			srv.password = *cliPwd
		} else {
			ensureFileExists("TLS client public key", *cliCCF, srv)
			ensureFileExists("TLS client private key", *cliCKF, srv)
			srv.Logger.Info("Loading TLS client certificate")
			cert, err := tls.LoadX509KeyPair(*cliCCF, *cliCKF)
			if err != nil {
				srv.Logger.Errorf("Failed to load TLS client certificate: %s", err)
				os.Exit(ExitArgument)
			}
			srv.clientCert = &cert
		}
	} else {
		// We are not running against a TLS secured endpoint, so print a warning if a client certificate
		// has been set. It's likely NOT what was intended.
		if *cliCCF != "" || *cliCKF != "" {
			srv.Logger.Warning("TLS authentication only available when connecting via TLS!")
		}

		// Check for regular ticket authentication.
		if *cliUsr == "" || *cliPwd == "" {
			srv.Logger.Warning("Missing username/password, disabling authentication completely.")
			srv.useAuth = false
		}
		srv.username = *cliUsr
		srv.password = *cliPwd
	}

	srv.Logger.Info("Hello")
	srv.Logger.Infof("%ssing TLS.", map[bool]string{true: "U", false: "Not u"}[srv.useTls])
	srv.Logger.Infof("Using '%s' as connection url...", srv.url)
	srv.Logger.Infof("Using '%s' as serialization type...", *cliSer)
	srv.Logger.Infof("Using '%s' as realm...", srv.realm)
	if !srv.useAuth {
		srv.Logger.Info("No authentication configured...")
	} else {
		if srv.username != "" && srv.password != "" {
			srv.Logger.Infof("Using '%s' as user-id...", srv.username)
		} else {
			srv.Logger.Info("Using TLS client authentication...")
		}
	}
	return srv
}

// Connect establishes a connection with the broker and must be called before `Run`!
//
// This function may exit the program early when
//
// 1. Logger creation failed.
//
// 2. The client failed to join the realm.
func (srv *Service) Connect() {
	var err error

	srv.Logger.Debug("Trying to connect to broker")
	var tlsCfg *tls.Config
	if srv.useTls {
		tlsCfg = &tls.Config{
			InsecureSkipVerify: false,
		}

		if srv.serverCert == nil {
			tlsCfg.InsecureSkipVerify = true
		} else {
			tlsCfg.RootCAs = srv.serverCert
		}

		if srv.clientCert != nil {
			tlsCfg.Certificates = append(tlsCfg.Certificates, *srv.clientCert)
		}
	}

	cfg := client.ClientConfig{
		Realm:           srv.realm,
		Serialization:   srv.serialization,
		ResponseTimeout: 5 * time.Second,
		TlsCfg:          tlsCfg,
	}

	if srv.useAuth {
		helloDetails := wamp.Dict{
			"authid": srv.username,
		}
		cfg.HelloDetails = helloDetails

		authMethods := make(map[string]client.AuthFunc)
		if srv.useTls && srv.clientCert != nil {
			authMethods["tls"] = func(_ *wamp.Challenge) (string, wamp.Dict) {
				return "", wamp.Dict{}
			}
		} else {
			authMethods["ticket"] = func(_ *wamp.Challenge) (string, wamp.Dict) {
				return srv.password, wamp.Dict{}
			}
		}
		cfg.AuthHandlers = authMethods
	}

	srv.Client, err = client.ConnectNet(srv.url, cfg)
	if err != nil {
		srv.Logger.Criticalf("Failed to connect service to broker: %s", err)
		os.Exit(ExitConnect)
	}
	srv.Logger.Info("Connected to broker")
}

// Run starts the microservice. This function blocks until the user interrupts the process
// with a SIGINT. It can be considered as the main loop of the service. This function may
// be only called once.
//
// This function can exit the program early when
//
// 1. The client failed to leave the realm.
//
// 2. The client connection failed to close.
func (srv *Service) Run() {
	defer srv.Client.Close()

	sigintChannel := make(chan os.Signal, 1)
	signal.Notify(sigintChannel, os.Interrupt)

	srv.Logger.Info("Entering main loop")
	fmt.Println("Send SIGINT to quit")
	select {
	case <-sigintChannel:
		// linebreak after echoed ^C
		fmt.Println()
		srv.Logger.Info("Received SIGINT, exiting")

	case <-srv.Client.Done():
		srv.Logger.Info("Connection lost, exiting")
	}
	srv.Logger.Info("Leaving main loop")
	srv.Logger.Info("Bye")
}

// RegistrationError describes an error that occurred during the registration of a remote procedure call.
// The struct holds the inner error and the procedure name that failed to register.
type RegistrationError struct {
	ProcedureName string
	Inner         error
}

// SubscriptionError describes an error that occurred during the subscription on a topic.
// The struct holds the inner error and the topic name that failed to subscribe.
type SubscriptionError struct {
	Topic string
	Inner error
}

// HandlerRegistration holds a tuple of a `client.InvocationHandler` and an options map
// that can be used in the `RegisterAll` function to register multiple method handlers
// at once.
type HandlerRegistration struct {
	Handler client.InvocationHandler
	Options wamp.Dict
}

// EventSubscription holds a tuple of a `client.EventHandler` and an options map
// that can be used in the `SubscribeAll` function to subcribe to multiple topics
// at once.
type EventSubscription struct {
	Handler client.EventHandler
	Options wamp.Dict
}

// RegisterAll can be used to register multiple remote procedure calls at once.
// You can use it like this:
//
// 	options := wamp.Dict{}
// 	procedures := map[string]service.HandlerRegistration{
// 		"example.get_magic":      service.HandlerRegistration{handler.GetMagic, options},
// 		"example.do_stuff":       service.HandlerRegistration{handler.DoStuff, options},
// 		"example.set_something":  service.HandlerRegistration{handler.SetSomething, options},
// 	}
// 	if err := util.App.RegisterAll(procedures); err != nil {
// 		util.Log.Criticalf(
// 			"Failed to register procedure '%s' in broker: %s",
// 			err.ProcedureName,
// 			err,
// 		)
// 		os.Exit(service.EXIT_REGISTRATION)
// 	}
func (srv *Service) RegisterAll(procedures map[string]HandlerRegistration) *RegistrationError {
	for name, regr := range procedures {
		if err := srv.Client.Register(name, regr.Handler, regr.Options); err != nil {
			return &RegistrationError{
				ProcedureName: name,
				Inner:         err,
			}
		}
	}

	return nil
}

// SubscribeAll can be used to subscribe to multiple topics at once.
// You can use it like this:
//
// 	options := wamp.Dict{}
// 	events := map[string]service.HandlerRegistration{
// 		"example.goo_happened":   service.EventSubscriptions{handler.GooHappened, options},
// 		"example.gesus_joined":   service.EventSubscriptions{handler.GesusJoined, options},
// 		"example.no_more_mate":   service.EventSubscriptions{handler.NoMoreMate, options},
// 	}
// 	if err := util.App.SubscribeAll(events); err != nil {
// 		util.Log.Criticalf(
// 			"Failed to subscribe to topic '%s' in broker: %s",
// 			err.Topic,
// 			err,
// 		)
// 		os.Exit(service.EXIT_REGISTRATION)
// 	}
func (srv *Service) SubscribeAll(events map[string]EventSubscription) *SubscriptionError {
	for topic, regr := range events {
		if err := srv.Client.Subscribe(topic, regr.Handler, regr.Options); err != nil {
			return &SubscriptionError{
				Topic: topic,
				Inner: err,
			}
		}
	}

	return nil
}

// ErrorKind describes the type of an error that occurred during the execution of the microservice.
// It can be used as a basic set of errors that are used by implementors of this service library.
type ErrorKind int

const (
	// ErrorBadArgument indicates that a given argument does not meet its requirements.
	ErrorBadArgument ErrorKind = iota

	// ErrorNotAvailable indicates that a requested resource is not available.
	ErrorNotAvailable

	// ErrorNotEnoughData indicates that the provided data is not enough.
	ErrorNotEnoughData

	// ErrorUnexpectedData indicates that the provided data is in an unexpected format.
	ErrorUnexpectedData

	// ErrorTooMuchData indicates that the provided data is too much.
	ErrorTooMuchData

	// ErrorOutOfRange indicates that a given index is out of range.
	ErrorOutOfRange

	// ErrorTimedOut indicates that a request has timed out.
	ErrorTimedOut

	// ErrorPermissionDenied indicates that the access to a resource was denied.
	ErrorPermissionDenied

	// ErrorNotFound indicates that a given resource could not be found.
	ErrorNotFound

	// ErrorUnreachableLineReached indicates that this code should not be reached as it is not implemented.
	ErrorUnreachableLineReached

	// ErrorThisWorksOnMyMachine indicates that this code needs complicated state to work. Contact your
	// system administrator for details.
	ErrorThisWorksOnMyMachine

	// ErrorItsNotABugItsAFeature indicates that the current behavior is intended. If you did not expect this to
	// happen, contact your system administrator.
	ErrorItsNotABugItsAFeature

	// ErrorAKittenDies indicates that something was nil...
	ErrorAKittenDies
)

// Error is the holder of an inner error and a translated `ErrorKind`.
// Instances may be created with `NewError` or `NewErrorFrom`.
type Error struct {
	kind  ErrorKind
	inner error
}

// NewError creates a new error from a given error kind.
func NewError(kind ErrorKind) *Error {
	return &Error{
		kind:  kind,
		inner: nil,
	}
}

// NewErrorFrom translates an inner to a given error kind and holds the
// inner error.
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
		msg = "The access to a resource was denied."
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
		msg = "Unknown error occurred."
	}

	if e.inner != nil {
		return fmt.Sprintf("%s\nInner Error: %s", msg, e.inner.Error())
	}
	return msg
}
