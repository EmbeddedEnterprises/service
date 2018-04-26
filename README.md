# service [![Latest Tag](https://img.shields.io/github/tag/EmbeddedEnterprises/service.svg)](https://github.com/EmbeddedEnterprises/service/releases) [![Build Status](https://travis-ci.org/EmbeddedEnterprises/service.svg?branch=master)](https://travis-ci.org/EmbeddedEnterprises/service) [![Go Report Card](https://goreportcard.com/badge/github.com/EmbeddedEnterprises/service)](https://goreportcard.com/report/github.com/EmbeddedEnterprises/service) [![GoDoc](https://godoc.org/github.com/EmbeddedEnterprises/service?status.svg)](https://godoc.org/github.com/EmbeddedEnterprises/service)

The robµlab service library is a convenience wrapper for easy microservice creation.

---

## How to add to your project?

Run this in your project

```
$ burrow get github.com/embeddedenterprises/service
```

and use the library in your sourcecode like this.

```go
package main

import (
	"os"

	"github.com/EmbeddedEnterprises/service"
	"github.com/jcelliott/turnpike"
	"github.com/op/go-logging"
)

func main() {
	srv := service.New(service.Config{
		Name:          "example",
		Serialization: turnpike.MSGPACK,
		Version:       "0.1.0",
		Description:   "Simple example microservice from the documentation.",
		URL:           "ws://localhost:8000/ws",
	})
	srv.Connect()

	// register and subscribe here

	srv.Run()
	os.Exit(service.ExitSuccess)
}
```

## How to view logging output

The robµlab service library uses the system logging daemon to store log files. You can use the following command under systemd machines to view a robµlab service's log messages:

```
$ journalctl --user -e -f -t com.robulab.<name>
```

## Running the example

First you have to start a crossbar broker in the background.

```
$ docker run -p 127.0.0.1:8080:8080 --name crossbar --rm crossbario/crossbar:latest
```

The you can run the example service like this:

```
$ burrow run --example simple -- -b ws://localhost:8080/ws
```

You can view the logging output of the example by issuing

```
$ journalctl --user -e -f -t com.robulab.example.simple
```
