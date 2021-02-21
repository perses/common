Common
======

This repository contains GO libraries used in the different component of Perses.

This repository is not thought to be used outside of Perses. Use it at your own risks.

* **app**: provides a struct to be used to help to start an application (usually with an HTTP API)
* **async**: provides different way to manage an asynchronous job
* **config**: provides a config resolver that helps to manage the configuration. It also provides a default
  configuration for etcd
* **echo**: provides a builder that helps to manage middleware, api and help to start a server with a context
  management.
* **etcd**: provides a dao that wraps the etcd client to simplify a bit how to use it
* **slices**: provides utils method to manipulate the slice (mostly slice of string)
