# service

[![build status](http://gitlab.pattig.rocks/robulab/Backend/service/badges/master/build.svg)](http://gitlab.pattig.rocks/robulab/Backend/service/commits/master)

The robµlab service library is a convenience wrapper for easy microservice creation.

---

## How to add to your project?

Run this in your project

```
$ burrow get github.com/embeddedenterprises/service
```

and import the library in your sourcecode like this.

```go
import "github.com/embeddedenterprises/service"
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
