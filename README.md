# service

[![build status](http://gitlab.pattig.rocks/robulab/Backend/service/badges/master/build.svg)](http://gitlab.pattig.rocks/robulab/Backend/service/commits/master)

The robµlab service library is a convenience wrapper for easy microservice creation.

---

## How to add to your project?

Unfortunately glide does not read `~/.gitconfig` for dependency resolving. Therefore we have to specify the import paths manually and cannot use `burrow get`.

Add this to your `glide.yaml` file in your project

```
import:
  - package: robulab/service
    repo: ssh://git@gitlab.development.coffee:1023/PKES-EE/Backend/service
    version: ^0.1.0
```

and run `burrow update` afterwards.

And in your source code you can now import the library.

```go
import "robulab/service"
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

## Gitconfig

Make sure your `~/.gitconfig` contains the following section

```
[url "ssh://git@gitlab.development.coffee:1023/PKES-EE/Backend"]
        insteadOf = git://robulab
```
