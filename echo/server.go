package echo

import (
	"context"
	"fmt"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/perses/common/async"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	persesMiddleware "github.com/perses/common/echo/middleware"
)

type Register interface {
	RegisterRoute(e *echo.Echo)
}

type Builder struct {
	metricNamespace    string
	addr               string
	apis               []Register
	overrideMiddleware bool
	mdws               []echo.MiddlewareFunc
}

func NewBuilder(addr string) *Builder {
	return &Builder{
		addr: addr,
	}
}

// Middleware is adding the provided middleware into the Builder
// Order matters, add the middleware in the order you would like to see them started.
func (b *Builder) Middleware(mdw echo.MiddlewareFunc) *Builder {
	b.mdws = append(b.mdws, mdw)
	return b
}

// OverrideDefaultMiddleware is setting a flag that will tell if the Builder needs to override the default list of middleware considered by the one provided by the method Middleware
// In case the flag is set at false, then the middleware provided by the user will be append to the default list.
// Note that the default list is always executed at the beginning (a.k.a, the default middleware will be executed before yours).
func (b *Builder) OverrideDefaultMiddleware(override bool) *Builder {
	b.overrideMiddleware = override
	return b
}

// MetricNamespace is modifying the namespace that will be used next ot prefix every metrics exposed
func (b *Builder) MetricNamespace(namespace string) *Builder {
	b.metricNamespace = namespace
	return b
}

func (b *Builder) APIRegistration(api Register) *Builder {
	b.apis = append(b.apis, api)
	return b
}

func (b *Builder) Build() (async.Task, error) {
	if len(b.apis) == 0 {
		return nil, fmt.Errorf("no api registered")
	}
	if !b.overrideMiddleware {
		defaultMiddleware := []echo.MiddlewareFunc{
			// Activate recover middleware to recover from panics anywhere in the chain.
			// It prints stack trace and handles the control to the centralized HTTPErrorHandler.
			// More information here: https://echo.labstack.com/middleware/recover
			middleware.Recover(),
			persesMiddleware.Logger(),
			middleware.GzipWithConfig(
				middleware.GzipConfig{
					Level: 5,
				},
			),
		}
		if len(b.metricNamespace) > 0 {
			metricMiddleware, err := persesMiddleware.NewMetrics(b.metricNamespace)
			if err != nil {
				return nil, err
			}
			prometheus.MustRegister(metricMiddleware)
			defaultMiddleware = append(defaultMiddleware, metricMiddleware.ProcessHTTPRequest)

		}
		b.mdws = append(defaultMiddleware, b.mdws...)
	}
	return &server{
		Task:            nil,
		addr:            b.addr,
		apis:            b.apis,
		e:               echo.New(),
		mdws:            b.mdws,
		shutdownTimeout: 30 * time.Second,
	}, nil
}

type server struct {
	async.Task
	addr            string
	apis            []Register
	e               *echo.Echo
	mdws            []echo.MiddlewareFunc
	shutdownTimeout time.Duration
}

func (s *server) String() string {
	return "http server"
}

func (s *server) Initialize() error {
	// init global middleware
	// Remove trailing slash middleware a trailing slash from the request URI
	s.e.Pre(middleware.RemoveTrailingSlash())
	for _, mdw := range s.mdws {
		s.e.Use(mdw)
	}
	// register apis
	for _, a := range s.apis {
		a.RegisterRoute(s.e)
	}
	return nil
}

func (s *server) Execute(ctx context.Context, cancelFunc context.CancelFunc) error {
	// start server
	serverCtx, serverCancelFunc := context.WithCancel(ctx)
	go func() {
		defer serverCancelFunc()
		if err := s.e.Start(s.addr); err != nil {
			logrus.WithError(err).Info("http server stopped")
		}
		logrus.Debug("go routine running the http server has been stopped.")
	}()
	// Wait for the end of the task or cancellation
	select {
	case <-serverCtx.Done():
		// Server ended unexpectedly
		// In our ecosystem, as we are producing each time an HTTP API, if the HTTP api stopped, we want to stop the whole application.
		// That's why we are calling the parent cancelFunc
		cancelFunc()
		// as it is possible that the serverCtx.Done() is closed because the main cancelFunc() has been called by another go routing,
		// we should try to close properly the http server
		// Note: that's why we don't return any error here.
	case <-ctx.Done():
		// Cancellation requested by the parent context
		logrus.Debug("server cancellation requested")
	}
	return nil
}

func (s *server) Finalize() error {
	logrus.Debug("try to shutdown the http server")
	shutdownCtx, shutdownCancelFunc := context.WithTimeout(context.Background(), s.shutdownTimeout)
	// call shutdownCancelFunc to release the resources in case the task ended before the timeout
	defer shutdownCancelFunc()
	if err := s.e.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown not properly: %w", err)
	}
	return nil
}
