Common
======
[![golangci-lint](https://github.com/perses/common/actions/workflows/golangci-lint.yml/badge.svg)](https://github.com/perses/common/actions/workflows/golangci-lint.yml)
[![Godoc](https://godoc.org/github.com/perses/common?status.svg)](https://pkg.go.dev/github.com/perses/common)

This repository contains GO libraries used in the different components of Perses.

This set of libraries aims to provide a way:

* to handle processes management with graceful stop
* to define an HTTP API
* to handle the configuration

Note: These libraries are mainly designed to ease the development of Perses. As it is still a beta, breaking change can
happen between two releases. But as there is nothing really specific to Perses itself, you can use it at your own
risk.

Here's a short description about what each package provides:

* **app**: provides a runner that starts an application (usually with an HTTP API) and its tasks, and stops them
  gracefully when the process receives SIGINT or SIGTERM.
* **async**: provides an Async/Await implementation, and the interfaces `SimpleTask` and `Task` to implement a
  long-running task. The subpackage **async/taskhelper** executes a task once, periodically or according to a cron
  schedule.
* **config**: provides a config resolver that reads the configuration from a YAML file and/or from the environment
  variables, verifies it, and can notify the changes of the file.
* **echo**: provides a builder that helps to manage middlewares, apis and help to start an HTTP or HTTPS server (based
  on [echo](https://echo.labstack.com)) with a context management, a `/metrics` endpoint and the access logs. The
  subpackage **echo/middleware** provides the logger and the metrics middlewares used by default.
* **file**: provides a way to watch a file and to be notified when it changes, including when it is a Kubernetes
  ConfigMap or Secret.
* **logrus**: provides a builder that configures the global [logrus](https://github.com/sirupsen/logrus) logger (level,
  format and method trace).
* **otel**: provides a builder that creates the OpenTelemetry tracer provider, registered globally when the application
  starts and shut down when it stops.
* **set**: provides a generic set, that can be marshalled to and unmarshalled from JSON and YAML as a sorted list.
* **slices**: provides utility methods to manipulate slices (mostly slices of string)

## Flags

The packages **app**, **echo** and **logrus** read their default configuration from command-line flags. These flags
must be registered before calling `flag.Parse`, with one of these functions:

* `app.InitFlag` registers all of them.
* `logrus.InitFlag` registers the flags `--log.*`, like `--log.level` and `--log.format`.
* `echo.InitFlag` registers the flags configuring the HTTP server, like `--web.listen-address`, and the HTTPS server,
  like `--web.tls-cert-file` and `--web.tls-key-file`.
* `echo.InitTelemetryPathFlag` registers the flag `--web.telemetry-path`.

The description of each flag is printed with `--help`.
