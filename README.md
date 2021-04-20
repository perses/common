Common
======
[![CircleCI](https://circleci.com/gh/perses/common.svg?style=shield)](https://circleci.com/gh/perses/common)
[![Godoc](https://godoc.org/github.com/perses/common?status.svg)](https://pkg.go.dev/github.com/perses/common)

This repository contains GO libraries used in the different component of Perses.

This set of library aims to provide a way:

* to handle processes management with graceful stop
* to defines an HTTP API
* to handle the configuration

Note: These libraries are mainly designed to ease the development of Perses. As it is still a beta, breaking change can
happen between two release. But, as there is nothing really specific to Perses itself, you can use it, but at your own
risks.

Here a short description about what each package provides.

* **app**: provides a struct to be used to help to start an application (usually with an HTTP API)
* **async**: provides different way to manage an asynchronous job
* **config**: provides a config resolver that helps to manage the configuration. It also provides a default
  configuration for etcd
* **echo**: provides a builder that helps to manage middleware, api and help to start a server with a context
  management.
* **etcd**: provides a dao that wraps the etcd client to simplify a bit how to use it
* **slices**: provides utils method to manipulate the slice (mostly slice of string)
