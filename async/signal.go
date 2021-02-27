// Copyright 2021 Amadeus s.a.s
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
