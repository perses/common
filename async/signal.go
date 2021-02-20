package async

import (
	"context"
	"os"
	"os/signal"

	"github.com/sirupsen/logrus"
)

type signalListener struct {
	SimpleTask
	signals []os.Signal
}

func NewSignalListener(signals ...os.Signal) SimpleTask {
	return &signalListener{
		signals: signals,
	}
}

func (s *signalListener) String() string {
	return "signal listener"
}

func (s *signalListener) Execute(ctx context.Context, cancelFunc context.CancelFunc) error {
	sigChannel := make(chan os.Signal, 1)
	signal.Notify(sigChannel, s.signals...)
	select {
	case sig := <-sigChannel:
		cancelFunc()
		logrus.Infof("signal received: %s", sig)
	case <-ctx.Done():
		logrus.Debugf("task '%s' has been canceled", s.String())
	}
	return nil
}
