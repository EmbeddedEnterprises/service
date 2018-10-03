# service [![Latest Tag](https://img.shields.io/github/tag/EmbeddedEnterprises/service.svg)](https://github.com/EmbeddedEnterprises/service/releases) [![Build Status](https://travis-ci.org/EmbeddedEnterprises/service.svg?branch=master)](https://travis-ci.org/EmbeddedEnterprises/service) [![Go Report Card](https://goreportcard.com/badge/github.com/EmbeddedEnterprises/service)](https://goreportcard.com/report/github.com/EmbeddedEnterprises/service) [![GoDoc](https://godoc.org/github.com/EmbeddedEnterprises/service?status.svg)](https://godoc.org/github.com/EmbeddedEnterprises/service)

The robµlab service library is a convenience wrapper for easy microservice creation.

---

## How to add to your project?

Run this in your project

```sh
$ burrow get github.com/embeddedenterprises/service
```

and use the library in your sourcecode like this.

```go
package main

import (
	"os"

	"github.com/EmbeddedEnterprises/service"
	"github.com/gammazero/nexus/client"
	"github.com/op/go-logging"
)

func main() {
	srv := service.New(service.Config{
		Name:          "example",
		Serialization: client.MSGPACK,
		Version:       "0.1.0",
		Description:   "Simple example microservice from the documentation.",
	})
	srv.Connect()

	// register and subscribe here

	srv.Run()
	os.Exit(service.ExitSuccess)
}
```

## Running the examples

### Simple example

First you have to start a WAMP router in the background (i.e. crossbar.io or nexus):

```sh
$ docker run -p 127.0.0.1:8080:8080 --name crossbar --rm crossbario/crossbar:latest
```

The you can run the example service like this:

```sh
$ burrow run --example simple -- -b ws://localhost:8080/ws -r realm1
```

### Authentication example

First you have to start a WAMP router configured with authentication in the background:

```sh
$ docker run -p 127.0.0.1:8080:8080 \
    --mount type=bind,source=$(pwd)/example/auth/crossbar.json,target=/node/.crossbar/config.json \
    --name crossbar --rm crossbario/crossbar:latest
```

Then you can run the auth example like this:

```sh
$ burrow run --example auth -- -b ws://localhost:8080/ws -u WRONG
# Should yield 'no such principal with authid WRONG'

$ burrow run --example auth -- -b ws://localhost:8080/ws -u CORRECT
# Should yield 'authentication failed'

$ burrow run --example auth -- -b ws://localhost:8080/ws -u CORRECT -p CORRECT
# Should work just like the 'simple' example.
```

The service library supports several authentication modes:

- Anonymous (i.e. no username and password is specified), the client is authenticated
  using other features, like remote-ip or some specific socket
- Ticket (normal username and password)
- TLS Client Auth, which provides encryption and authentication utilizing a PKI.
