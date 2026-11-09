// Copyright The Perses Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package echo is exposing a struct to handle the building and the management of the different tasks coming from the async package.
// This should be used in the main package only.
// This package provides a way to build an echo server easily (see https://echo.labstack.com), with a prometheus metrics endpoint and that relies on logrus for logging (see https://github.com/sirupsen/logrus).
//
// Please favor the usage of [app](../app) package to run an echo web server.
//
// # Features
//
// - Build and run an echo server with a "/metrics" endpoint.
//
// - Register an API.
//
// - Register a Middleware.
//
// # Usage
//
// Instantiate a simple server task :
//
//	package my_package
//
//	import (
//	    "context"
//
//	    "github.com/perses/common/echo"
//	)
//
//	const (
//	    // The address on which the server is listening.
//	    addr = ":8080"
//	    metricNamespace = "my_project"
//	)
//
//	func main() {
//	    serverTask, err := echo.NewBuilder(addr).
//	            APIRegistration(echo.NewMetricsAPI(true)).
//	            MetricNamespace(metricNamespace).
//	            Build()
//	}
package echo

import (
	"cmp"
	"context"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"slices"
	"strings"
	"time"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
	"github.com/perses/common/async"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors/version"
	"github.com/sirupsen/logrus"

	persesMiddleware "github.com/perses/common/echo/middleware"
)

const defaultAddr = ":8080"

const (
	// defaultReadHeaderTimeout protects the server against clients sending the request headers slowly to keep the connections open (Slowloris attack).
	defaultReadHeaderTimeout = 10 * time.Second
	// defaultIdleTimeout closes the keep-alive connections that are not used anymore.
	defaultIdleTimeout = 2 * time.Minute
)

var (
	// http address listened
	addr string
	// hide the address listened when logging that the server started
	hidePort bool
	// https cert for server
	cert string
	// https key for server
	key string
	// TLS minimum version (e.g., "1.2", "1.3")
	tlsMinVersion string
	// TLS maximum version (e.g., "1.2", "1.3")
	tlsMaxVersion string
	// TLS cipher suites (comma-separated list of cipher suite names)
	tlsCipherSuites string
)

func InitFlag() {
	flag.StringVar(&addr, "web.listen-address", defaultAddr, "The address to listen on for HTTP requests, web interface and telemetry.")
	flag.BoolVar(&hidePort, "web.hide-port", false, "If true, the address listened to receive the HTTP requests is not logged when the server starts")
	flag.StringVar(&cert, "web.tls-cert-file", "", "The path to the cert to use for the HTTPS server. It is reloaded when the file changes.")
	flag.StringVar(&key, "web.tls-key-file", "", "The path to the key to use for the HTTPS server. It is reloaded when the file changes.")
	flag.StringVar(&tlsMinVersion, "web.tls-min-version", "", "Minimum TLS version (e.g., \"1.2\", \"1.3\")")
	flag.StringVar(&tlsMaxVersion, "web.tls-max-version", "", "Maximum TLS version (e.g., \"1.2\", \"1.3\")")
	flag.StringVar(&tlsCipherSuites, "web.tls-cipher-suites", "", "Comma-separated list of TLS cipher suite names")
}

// parseTLSVersion converts a version string (e.g., "1.2", "1.3") to a tls.Version constant.
// Returns 0 for empty string (not set), or an error for unknown versions.
func parseTLSVersion(version string) (uint16, error) {
	if version == "" {
		return 0, nil
	}
	switch version {
	case "1.0":
		return tls.VersionTLS10, nil
	case "1.1":
		return tls.VersionTLS11, nil
	case "1.2":
		return tls.VersionTLS12, nil
	case "1.3":
		return tls.VersionTLS13, nil
	default:
		return 0, fmt.Errorf("unknown TLS version %q, valid options are: 1.0, 1.1, 1.2, 1.3", version)
	}
}

// tlsVersionToString converts a tls.Version constant to its string representation.
func tlsVersionToString(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "1.0"
	case tls.VersionTLS11:
		return "1.1"
	case tls.VersionTLS12:
		return "1.2"
	case tls.VersionTLS13:
		return "1.3"
	default:
		return "unknown"
	}
}

// parseCipherSuites converts a comma-separated list of cipher suite names to their IDs.
// Returns nil for empty string (not set), or an error for unknown cipher suite names.
func parseCipherSuites(suites string) ([]uint16, error) {
	if suites == "" {
		return nil, nil
	}

	// Build a map of cipher suite names to IDs
	cipherSuiteMap := make(map[string]uint16)
	for _, cs := range tls.CipherSuites() {
		cipherSuiteMap[cs.Name] = cs.ID
	}

	var result []uint16
	for name := range strings.SplitSeq(suites, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		id, ok := cipherSuiteMap[name]
		if !ok {
			return nil, fmt.Errorf("unknown cipher suite %q", name)
		}
		result = append(result, id)
	}
	return result, nil
}

type Register interface {
	RegisterRoute(e *echo.Echo)
}

type Builder struct {
	metricNamespace    string
	promRegisterer     prometheus.Registerer
	addr               string
	apis               []Register
	overrideMiddleware bool
	mdws               []echo.MiddlewareFunc
	preMDWs            []echo.MiddlewareFunc
	gzipSkipper        middleware.Skipper
	activatePprof      bool
	readHeaderTimeout  time.Duration
	readTimeout        time.Duration
	writeTimeout       time.Duration
	idleTimeout        time.Duration
	ipExtractor        echo.IPExtractor
	schemeExtractor    echo.SchemeExtractor
}

func NewBuilder() *Builder {
	return &Builder{}
}

func (b *Builder) ListenAddress(addr string) {
	b.addr = addr
}

// PreMiddleware is adding the provided middleware into the Builder.
// Each mdw added here, will be executed before the router.
func (b *Builder) PreMiddleware(mdw echo.MiddlewareFunc) *Builder {
	b.preMDWs = append(b.preMDWs, mdw)
	return b
}

// Middleware is adding the provided middleware into the Builder
// Order matters, add the middleware in the order you would like to see them started.
func (b *Builder) Middleware(mdw echo.MiddlewareFunc) *Builder {
	b.mdws = append(b.mdws, mdw)
	return b
}

// OverrideDefaultMiddleware is setting a flag that will tell if the Builder needs to override the default list of middleware considered by the one provided by the method Middleware.
// In case the flag is set at false, then the middleware provided by the user will be appended to the default list.
// Note that the default list is always executed at the beginning (a.k.a, the default middleware will be executed before yours).
func (b *Builder) OverrideDefaultMiddleware(override bool) *Builder {
	b.overrideMiddleware = override
	return b
}

// GzipSkipper can be used to provide a function that will tell when to skip the gzip compression.
// It can be used to avoid gzip to certain URL(s).
// The Gzip compression is activated on every URL registered in echo when using the default middleware.
// If you don't use the default middleware, then there is no point of using this method.
func (b *Builder) GzipSkipper(skipper middleware.Skipper) *Builder {
	b.gzipSkipper = skipper
	return b
}

// MetricNamespace is modifying the namespace that will be used next ot prefix every metrics exposed
func (b *Builder) MetricNamespace(namespace string) *Builder {
	b.metricNamespace = namespace
	return b
}

// PrometheusRegisterer will set a new metric registry for Prometheus, so it won't use the default one.
// That can be useful for testing purpose since you can't register in the same go instance the same metrics multiple times.
func (b *Builder) PrometheusRegisterer(r prometheus.Registerer) *Builder {
	b.promRegisterer = r
	return b
}

// APIRegistration must be used to register an HTTP API.
func (b *Builder) APIRegistration(api Register) *Builder {
	b.apis = append(b.apis, api)
	return b
}

// ActivatePprof exposes the pprof endpoints under /debug/pprof. It is disabled by default.
// These endpoints are not authenticated and expose sensitive information about the process (like the command line arguments, the memory or the goroutine stacks),
// so only activate them when the server is not reachable by untrusted clients.
//
// Note: this package imports net/http/pprof, which registers the pprof endpoints on http.DefaultServeMux.
// So, whatever this option is, never serve http.DefaultServeMux (for example with http.ListenAndServe(addr, nil)) to untrusted clients.
func (b *Builder) ActivatePprof(activate bool) *Builder {
	b.activatePprof = activate
	return b
}

// ReadHeaderTimeout sets the amount of time allowed to read the request headers (see http.Server.ReadHeaderTimeout).
// It protects the server against clients sending the headers slowly to keep the connections open (Slowloris attack).
// The default value is 10s, or the ReadTimeout if it is shorter. A negative value disables the timeout.
func (b *Builder) ReadHeaderTimeout(timeout time.Duration) *Builder {
	b.readHeaderTimeout = timeout
	return b
}

// ReadTimeout sets the maximum duration to read the entire request, including the body (see http.Server.ReadTimeout).
// There is no timeout by default, as a request body can be large.
func (b *Builder) ReadTimeout(timeout time.Duration) *Builder {
	b.readTimeout = timeout
	return b
}

// WriteTimeout sets the maximum duration before timing out the writes of the response (see http.Server.WriteTimeout).
// There is no timeout by default, as some responses can take a long time (long queries, streaming, CPU profiles, etc.).
func (b *Builder) WriteTimeout(timeout time.Duration) *Builder {
	b.writeTimeout = timeout
	return b
}

// IdleTimeout sets the maximum amount of time to wait for the next request when keep-alives are enabled (see http.Server.IdleTimeout).
// The default value is 2m, whatever the ReadTimeout is. A negative value disables the timeout.
func (b *Builder) IdleTimeout(timeout time.Duration) *Builder {
	b.idleTimeout = timeout
	return b
}

// IPExtractor sets how echo.Context.RealIP gets the IP address of the client.
// By default, the address of the direct peer (http.Request.RemoteAddr) is used, as the headers X-Forwarded-For and X-Real-IP
// can be spoofed by any client. When the server is behind proxies, use an extractor that only trusts the headers set by them,
// for example:
//
//	builder.IPExtractor(echo.ExtractIPFromXFFHeader(echo.TrustIPRange(proxyNetwork)))
func (b *Builder) IPExtractor(extractor echo.IPExtractor) *Builder {
	b.ipExtractor = extractor
	return b
}

// SchemeExtractor sets how echo.Context.Scheme gets the scheme (http or https) used by the client.
// By default, the headers like X-Forwarded-Proto are only trusted when the request comes from a loopback, link-local or
// private network address. When the server is behind a proxy with another address (like a CDN or a cloud load balancer),
// use an extractor that trusts it, for example:
//
//	builder.SchemeExtractor(echo.ExtractSchemeFromHeaders(echo.TrustIPRange(proxyNetwork)))
func (b *Builder) SchemeExtractor(extractor echo.SchemeExtractor) *Builder {
	b.schemeExtractor = extractor
	return b
}

func (b *Builder) Build() (async.Task, error) {
	return b.build()
}

// BuildHandler is creating an http Handler based on the different configuration and attribute set.
// It can be useful to have it when you want to use the method httptest.NewServer for testing purpose, and you want to have the same setup as the actual http server.
func (b *Builder) BuildHandler() (http.Handler, error) {
	s, err := b.build()
	if err != nil {
		return nil, err
	}

	err = s.Initialize()
	return s.e, err
}

func (b *Builder) build() (*server, error) {
	if len(b.apis) == 0 {
		return nil, fmt.Errorf("no api registered")
	}
	// Serving HTTP when HTTPS is expected would expose the traffic, so a partial TLS configuration is rejected.
	// It is checked before anything is modified or registered.
	switch {
	case cert != "" && key == "":
		return nil, errors.New("--web.tls-key-file must be set when --web.tls-cert-file is set")
	case cert == "" && key != "":
		return nil, errors.New("--web.tls-cert-file must be set when --web.tls-key-file is set")
	}
	if len(b.addr) == 0 {
		b.addr = addr
		if len(addr) == 0 {
			b.addr = defaultAddr
		}
	}
	// The middlewares of the Builder are not modified, so Build can be called several times.
	mdws := slices.Clone(b.mdws)
	if !b.overrideMiddleware {
		if b.gzipSkipper == nil {
			b.gzipSkipper = middleware.DefaultSkipper
		}
		// The logger and the metrics are executed first, so they see the result of every request.
		// It includes the panics, as the middleware Recover turns them into errors returned to the previous middlewares.
		defaultMiddleware := []echo.MiddlewareFunc{persesMiddleware.Logger()}
		if b.promRegisterer == nil {
			b.promRegisterer = prometheus.DefaultRegisterer
		}
		if len(b.metricNamespace) > 0 {
			metricMiddleware, err := b.registerMetrics()
			if err != nil {
				return nil, err
			}
			defaultMiddleware = append(defaultMiddleware, metricMiddleware.ProcessHTTPRequest)
		}
		defaultMiddleware = append(defaultMiddleware,
			// Recover from the panics anywhere in the rest of the chain.
			// The panic is returned as an error (with the stack trace) that is logged by the central error handler.
			// Only the stack of the goroutine that panicked is kept: getting the stacks of all the goroutines stops the world.
			// More information here: https://echo.labstack.com/docs/middleware/recover
			middleware.RecoverWithConfig(middleware.RecoverConfig{DisableStackAll: true}),
			middleware.GzipWithConfig(
				middleware.GzipConfig{
					Skipper: b.gzipSkipper,
					Level:   5,
				},
			),
		)
		mdws = append(defaultMiddleware, mdws...)
	}

	// Parse TLS options from flags
	parsedMinVersion, err := parseTLSVersion(tlsMinVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid TLS min version: %w", err)
	}
	parsedMaxVersion, err := parseTLSVersion(tlsMaxVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid TLS max version: %w", err)
	}
	parsedCipherSuites, err := parseCipherSuites(tlsCipherSuites)
	if err != nil {
		return nil, fmt.Errorf("invalid TLS cipher suites: %w", err)
	}

	// Validate that min version is not greater than max version
	if parsedMinVersion != 0 && parsedMaxVersion != 0 && parsedMinVersion > parsedMaxVersion {
		return nil, fmt.Errorf("TLS min version (%s) cannot be greater than max version (%s)",
			tlsVersionToString(parsedMinVersion), tlsVersionToString(parsedMaxVersion))
	}
	if cert == "" && (tlsMinVersion != "" || tlsMaxVersion != "" || tlsCipherSuites != "") {
		logrus.Warn("--web.tls-min-version, --web.tls-max-version and --web.tls-cipher-suites have no effect because HTTPS is not enabled (--web.tls-cert-file and --web.tls-key-file are not set)")
	}

	e := echo.NewWithConfig(echo.Config{
		// echo logs with log/slog: its logs are written with logrus, like the other logs.
		Logger:           slog.New(logrusHandler{}),
		HTTPErrorHandler: handleError,
		IPExtractor:      b.ipExtractor,
		SchemeExtractor:  b.schemeExtractor,
	})
	readHeaderTimeout := b.readHeaderTimeout
	if readHeaderTimeout == 0 {
		readHeaderTimeout = defaultReadHeaderTimeout
		// Like http.Server, the headers cannot take longer to be read than the whole request.
		if b.readTimeout > 0 {
			readHeaderTimeout = min(b.readTimeout, defaultReadHeaderTimeout)
		}
	}
	return &server{
		addr:              b.addr,
		apis:              b.apis,
		e:                 e,
		cert:              cert,
		key:               key,
		tlsMinVersion:     parsedMinVersion,
		tlsMaxVersion:     parsedMaxVersion,
		tlsCipherSuites:   parsedCipherSuites,
		mdws:              mdws,
		preMDWs:           b.preMDWs,
		shutdownTimeout:   30 * time.Second,
		activatePprof:     b.activatePprof,
		readHeaderTimeout: readHeaderTimeout,
		readTimeout:       b.readTimeout,
		writeTimeout:      b.writeTimeout,
		idleTimeout:       cmp.Or(b.idleTimeout, defaultIdleTimeout),
	}, nil
}

// registerMetrics registers the metrics of the HTTP server.
// If they are already registered (for example, when Build is called several times), the existing ones are used.
func (b *Builder) registerMetrics() (*persesMiddleware.Metrics, error) {
	metricMiddleware, err := persesMiddleware.NewMetrics(b.metricNamespace)
	if err != nil {
		return nil, err
	}
	if err = b.promRegisterer.Register(metricMiddleware); err != nil {
		var alreadyRegistered prometheus.AlreadyRegisteredError
		if !errors.As(err, &alreadyRegistered) {
			return nil, fmt.Errorf("unable to register the HTTP metrics: %w", err)
		}
		existing, ok := alreadyRegistered.ExistingCollector.(*persesMiddleware.Metrics)
		if !ok {
			return nil, fmt.Errorf("unable to register the HTTP metrics: %w", err)
		}
		metricMiddleware = existing
	}
	if err = b.promRegisterer.Register(version.NewCollector(b.metricNamespace)); err != nil && !errors.As(err, new(prometheus.AlreadyRegisteredError)) {
		return nil, fmt.Errorf("unable to register the version metric: %w", err)
	}
	return metricMiddleware, nil
}

type server struct {
	async.Task
	addr              string
	apis              []Register
	e                 *echo.Echo
	cert              string
	key               string
	tlsMinVersion     uint16
	tlsMaxVersion     uint16
	tlsCipherSuites   []uint16
	mdws              []echo.MiddlewareFunc
	preMDWs           []echo.MiddlewareFunc
	shutdownTimeout   time.Duration
	activatePprof     bool
	readHeaderTimeout time.Duration
	readTimeout       time.Duration
	writeTimeout      time.Duration
	idleTimeout       time.Duration
	// shutdownErr is the error of the graceful shutdown done by Execute (like a timeout), returned by Finalize.
	shutdownErr error
}

func (s *server) String() string {
	return "http server"
}

func (s *server) Initialize() error {
	// init global middleware
	// Remove trailing slash middleware a trailing slash from the request URI
	s.e.Pre(middleware.RemoveTrailingSlash())
	for _, p := range s.preMDWs {
		s.e.Pre(p)
	}
	for _, mdw := range s.mdws {
		s.e.Use(mdw)
	}
	// register apis
	for _, a := range s.apis {
		a.RegisterRoute(s.e)
	}
	s.registerPprof()
	return nil
}

func (s *server) Execute(ctx context.Context, cancelFunc context.CancelFunc) error {
	startConfig := echo.StartConfig{
		Address:    s.addr,
		HideBanner: true,
		// The address is logged by logListenerAddr instead of echo, to tell if the server uses HTTPS (like echo v4).
		HidePort:         true,
		ListenerAddrFunc: s.logListenerAddr,
		GracefulTimeout:  s.shutdownTimeout,
		// echo logs this error and returns nil. It is returned by Finalize instead, so the task reports the failure.
		// It is called before Start returns, so before Finalize is called.
		OnShutdownError: func(err error) {
			s.shutdownErr = err
		},
		BeforeServeFunc: func(httpServer *http.Server) error {
			s.configureHTTPServer(httpServer)
			return nil
		},
	}
	if s.cert != "" && s.key != "" {
		tlsConfig, err := s.tlsConfig()
		if err != nil {
			cancelFunc()
			return fmt.Errorf("http server stopped unexpectedly: %w", err)
		}
		startConfig.TLSConfig = tlsConfig
	}
	// Start blocks until the context is canceled, then it gracefully shuts down the server.
	// It returns before if the server cannot start or stops unexpectedly (for example, when the address is already in use).
	err := startConfig.Start(ctx, s.e)
	logrus.Debug("http server has been stopped")
	if ctx.Err() == nil {
		// In our ecosystem, as we are producing each time an HTTP API, if the HTTP api stopped, we want to stop the whole application.
		// That's why we are calling the parent cancelFunc
		cancelFunc()
	}
	if err != nil {
		return fmt.Errorf("http server stopped unexpectedly: %w", err)
	}
	return nil
}

// configureHTTPServer sets the timeouts and the error logger of the http.Server created by echo.
// Every timeout is set, even when it is zero, as echo sets a default ReadTimeout.
func (s *server) configureHTTPServer(httpServer *http.Server) {
	httpServer.ReadHeaderTimeout = s.readHeaderTimeout
	httpServer.ReadTimeout = s.readTimeout
	httpServer.WriteTimeout = s.writeTimeout
	httpServer.IdleTimeout = s.idleTimeout
	// echo logs every error of the server as an error, but most of them are caused by the clients (see httpServerErrorWriter).
	httpServer.ErrorLog = log.New(httpServerErrorWriter{}, "", 0)
}

// tlsConfig builds the TLS configuration of the HTTPS server.
// The certificate is provided by a reloader, so a rotated certificate is used without restarting the server.
func (s *server) tlsConfig() (*tls.Config, error) {
	reloader, err := newCertificateReloader(s.cert, s.key)
	if err != nil {
		return nil, err
	}
	tlsConfig := &tls.Config{
		MinVersion:     s.tlsMinVersion,
		MaxVersion:     s.tlsMaxVersion,
		CipherSuites:   s.tlsCipherSuites,
		GetCertificate: reloader.GetCertificate,
	}
	if isHTTP2Compatible(s.tlsMinVersion, s.tlsCipherSuites) {
		tlsConfig.NextProtos = []string{"h2"}
	} else {
		logrus.Warn("HTTP/2 is disabled because --web.tls-cipher-suites contains neither TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256 nor TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, one of which HTTP/2 requires with TLS 1.2 (RFC 7540, section 9.2.2)")
	}
	return tlsConfig, nil
}

// isHTTP2Compatible tells if HTTP/2 can be offered with the given TLS settings.
// With TLS 1.2, HTTP/2 deployments must support TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256 (RFC 7540, section 9.2.2), so every
// HTTP/2 client can negotiate a cipher suite allowed by HTTP/2 (the connections using a cipher suite forbidden by
// RFC 7540, appendix A, are rejected). net/http checks it only when the TLS configuration is set on the http.Server,
// not on the listener like here. So, like net/http, HTTP/2 is only offered when TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256
// or TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256 is configured.
func isHTTP2Compatible(minVersion uint16, cipherSuites []uint16) bool {
	if len(cipherSuites) == 0 || minVersion >= tls.VersionTLS13 {
		return true
	}
	return slices.ContainsFunc(cipherSuites, func(id uint16) bool {
		return id == tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256 ||
			id == tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256
	})
}

func (s *server) Finalize() error {
	// The server is gracefully stopped by Execute once the context is canceled.
	if s.shutdownErr != nil {
		return fmt.Errorf("server shutdown not properly: %w", s.shutdownErr)
	}
	return nil
}

func (s *server) registerPprof() {
	if s.activatePprof {
		// The handlers are registered explicitly instead of delegating to http.DefaultServeMux,
		// so any other handler registered on it is not exposed.
		s.e.GET("/debug/pprof", echo.WrapHandler(http.HandlerFunc(pprof.Index)))
		s.e.GET("/debug/pprof/cmdline", echo.WrapHandler(http.HandlerFunc(pprof.Cmdline)))
		s.e.GET("/debug/pprof/profile", echo.WrapHandler(http.HandlerFunc(pprof.Profile)))
		s.e.GET("/debug/pprof/symbol", echo.WrapHandler(http.HandlerFunc(pprof.Symbol)))
		s.e.GET("/debug/pprof/trace", echo.WrapHandler(http.HandlerFunc(pprof.Trace)))
		// pprof.Index serves the named profiles (heap, goroutine, allocs, ...) under /debug/pprof/<name>.
		s.e.GET("/debug/pprof/*", echo.WrapHandler(http.HandlerFunc(pprof.Index)))
	}
}
