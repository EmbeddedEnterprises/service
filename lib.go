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
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"reflect"
	"strings"
	"time"

	"github.com/gammazero/nexus/client"
	"github.com/gammazero/nexus/transport/serialize"
	"github.com/gammazero/nexus/wamp"
	"github.com/mitchellh/mapstructure"
	flag "github.com/ogier/pflag"
	"github.com/op/go-logging"
)

// BinaryDataExtension is the extension number used to correctly encode raw binary
// data in msgpack. This is required because most msgpack implementations don't
// distinguish between strings and byte arrays, but JSON does, which leads to invalid
// utf-8 characters in JSON strings.
// This extension allows us to pass binary messages from a msgpack client to a
// JSON client.
const BinaryDataExtension byte = 42

func init() {
	encode := func(value reflect.Value) ([]byte, error) {
		return value.Bytes(), nil
	}
	decode := func(value reflect.Value, data []byte) error {
		value.Elem().SetBytes(data)
		return nil
	}
	serialize.MsgpackRegisterExtension(reflect.TypeOf(serialize.BinaryData{}), 42, encode, decode)
}

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

// EnvConnectTimeout defines the environment variable name for the connect timeout definition.
const EnvConnectTimeout string = "SERVICE_CONNECT_TIMEOUT"

// EnvTLSClientCertFile defines the environment variable name for the TLS client certificate
// public key to present to the router.
const EnvTLSClientCertFile string = "TLS_CLIENT_CERT"

// EnvTLSClientKeyFile defines the environment variable name for the TLS client certificate
// private key to present to the router.
const EnvTLSClientKeyFile string = "TLS_CLIENT_KEY"

// EnvTLSServerCertFile defines the environment variable name for the TLS server certificate
// public key to verify the server certificate against.
const EnvTLSServerCertFile string = "TLS_SERVER_CERT"

// EnvPingEnabled defines the environment variable name for the flag indicating
// whether server ping should be enabled
const EnvPingEnabled string = "SERVICE_ENABLE_PING"

// EnvPingInterval defines the environment variable name for the ping interval definition
const EnvPingInterval string = "SERVICE_PING_INTERVAL"

// EnvPingEndpoint defines the environment variable name for the ping procedure to call
const EnvPingEndpoint string = "SERVICE_PING_ENDPOINT"

// Version defines the git tag this code is built with
const Version string = "0.15.0"

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
	pingEnabled   bool
	pingInterval  time.Duration
	pingEndpoint  string
	useAuth       bool
	useTLS        bool
	serverCert    *x509.CertPool
	clientCert    *tls.Certificate
	Logger        *logging.Logger
	Client        *client.Client
	timeout       time.Duration
}

// Config is a structure describing the service. It is used to describe the service
// when running with --version or --help.
// Values passed in the config structure can't be overridden at runtime.
type Config struct {
	Name          string
	Version       string
	Description   string
	Serialization serialize.Serialization
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

	// build the command line interface, allow to override the values provided by the environment
	var cliVer = flag.BoolP("version", "V", false, "prints the version")
	var cliURL = flag.StringP("broker-url", "b", os.Getenv(EnvBrokerURL), "the websocket url of the broker")
	var cliUsr = flag.StringP("user", "u", os.Getenv(EnvUsername), "the user to login as")
	var cliPwd = flag.StringP("password", "p", os.Getenv(EnvPassword), "the password to login with")
	var cliRlm = flag.StringP("realm", "r", os.Getenv(EnvRealm), "the name of the realm to connect to")
	var cliCCF = flag.String("tls-client-cert-file", os.Getenv(EnvTLSClientCertFile), "TLS client public key file")
	var cliCKF = flag.String("tls-client-key-file", os.Getenv(EnvTLSClientKeyFile), "TLS client private key file")
	var cliSCF = flag.String("tls-server-cert-file", os.Getenv(EnvTLSServerCertFile), "TLS server public key file")
	var cliTimeout = flag.String("connect-timeout", os.Getenv(EnvConnectTimeout), "Timeout for broker connection, 0s to use default")
	_, enablePing := os.LookupEnv(EnvPingEnabled)
	var pingEnable = flag.Bool("ping-enable", enablePing, "Whether to send a ping to the server")
	var pingEndpoint = flag.String("ping-endpoint", os.Getenv(EnvPingEndpoint), "Which procedure to call when pinging the server")
	var pingInterval = flag.String("ping-interval", os.Getenv(EnvPingInterval), "Duration between two pings")
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
	srv.pingEnabled = true
	srv.pingEndpoint = "ee.ping"
	srv.pingInterval = 10 * time.Second

	setupLogger(srv)
	srv.serialization = defaultConfig.Serialization

	if *cliURL == "" {
		srv.Logger.Error("Please provide a broker url!")
		flag.Usage()
		os.Exit(ExitArgument)
	}

	if *cliRlm == "" {
		srv.Logger.Error("Please provide a realm!")
		flag.Usage()
		os.Exit(ExitArgument)
	}

	if !*pingEnable {
		srv.pingEnabled = false
	}
	if *pingEndpoint != "" {
		srv.pingEndpoint = *pingEndpoint
	}

	if pingIntervalDur, err := time.ParseDuration(*pingInterval); err != nil || pingIntervalDur < 1*time.Second {
		srv.Logger.Warningf("Ping interval '%s' is invalid: %v", *pingInterval, err)
		srv.Logger.Warningf("Falling back to 10s")
		srv.pingInterval = 10 * time.Second
	} else {
		srv.pingInterval = pingIntervalDur
	}

	// setup the final values to use for this service
	srv.url = *cliURL
	srv.realm = *cliRlm
	timeout, err := time.ParseDuration(*cliTimeout)
	if err != nil {
		srv.Logger.Error("Specified timeout '%s' is invalid!", *cliTimeout)
		flag.Usage()
		os.Exit(ExitArgument)
	}
	if timeout != 0 && timeout < 1*time.Second {
		srv.Logger.Info("Setting timeout to '1s', specifed duration was too short")
		timeout = 1 * time.Second
	}
	srv.timeout = timeout
	srv.useAuth = true

	// when wss:// is set, we are using TLS to secure the connection.
	if strings.HasPrefix(srv.url, "wss://") {
		srv.useTLS = true

		// Check whether the user requested to validate the servers identity
		// If so, check the file exists and is a valid certificate
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

		// Check whether the user requested to authenticate the service using TLS client certificates
		// If so, check the certificates exist and are valid
		if *cliCCF == "" || *cliCKF == "" {
			// Otherwise, fallback to username/password
			srv.Logger.Info("TLS client certificate not provided, falling back to ticket auth")
			srv.clientCert = nil
			if *cliUsr == "" || *cliPwd == "" {
				// Fallback to anonymous
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
	srv.Logger.Infof("%ssing TLS.", map[bool]string{true: "U", false: "Not u"}[srv.useTLS])
	srv.Logger.Infof("Using '%s' as connection url...", srv.url)
	srv.Logger.Infof("Using '%s' as serialization type...", srv.serialization)
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
	if srv.useTLS {
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

	if srv.timeout > time.Duration(0) {
		cfg.Dial = func(network, address string) (net.Conn, error) {
			return net.DialTimeout(network, address, srv.timeout)
		}
	}

	if srv.useAuth {
		helloDetails := wamp.Dict{
			"authid": srv.username,
		}
		cfg.HelloDetails = helloDetails

		authMethods := make(map[string]client.AuthFunc)
		if srv.useTLS && srv.clientCert != nil {
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

	pingClose := make(chan struct{}, 1)

	if srv.pingEnabled {
		go srv.runPing(pingClose)
	}

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
	close(pingClose)
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

func (srv *Service) runPing(closePing chan struct{}) {
	ticker := time.NewTicker(srv.pingInterval)
outer:
	for {
		select {
		case <-closePing:
			break outer
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), srv.pingInterval)
			if _, err := srv.Client.Call(ctx, srv.pingEndpoint, nil, nil, nil, ""); err != nil {
				cancel()
				srv.Logger.Criticalf("Ping failed, exiting! %v", err)
				srv.Client.Close()
				break outer
			}
			cancel()
		}
	}
}

// ReturnValue constructs a wamp response which contains just one arbitrary value.
// Its primary use is to save boilerplate code.
func ReturnValue(value interface{}) *client.InvokeResult {
	return &client.InvokeResult{
		Args: wamp.List{
			value,
		},
	}
}

// ReturnError constructs a wamp response which contains an error with the specified URI.
// Its primary use is to save boilerplate code.
func ReturnError(uri string) *client.InvokeResult {
	return &client.InvokeResult{
		Err: wamp.URI(uri),
	}
}

// ReturnEmpty constructs an empty wamp response, the equivalent of void.
// Its primary use is to save boilerplate code.
func ReturnEmpty() *client.InvokeResult {
	return &client.InvokeResult{}
}

// IsRPCError checks whether the given error is a wamp RPC error
func IsRPCError(err error) bool {
	_, ok := err.(client.RPCError)
	return ok
}

// IsSpecificRPCError checks whether the given error is a wamp RPC error witch the expected error URI
func IsSpecificRPCError(err error, uri wamp.URI) bool {
	rpc, ok := err.(client.RPCError)
	return ok && rpc.Err.Error == uri
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

// CallerID represents a caller of a wamp RPC invocation
type CallerID struct {
	Session  wamp.ID  `mapstructure:"caller"`
	Username string   `mapstructure:"caller_authid"`
	Role     []string `mapstructure:"caller_authrole"`
}

// ParseCallerID extracts caller information from the details dictionary of a wamp RPC invocation
func ParseCallerID(details wamp.Dict) (*CallerID, error) {
	c := &CallerID{}
	if err := mapstructure.WeakDecode(details, c); err != nil {
		return nil, err
	}
	return c, nil
}

// HasAnyRole checks whether the caller object has any of the specified roles
func (c *CallerID) HasAnyRole(test []string) bool {
	for _, r := range c.Role {
		for _, r2 := range test {
			if r == r2 {
				return true
			}
		}
	}
	return false
}
