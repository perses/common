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

package logrus

import (
	"flag"

	"github.com/sirupsen/logrus"
)

type format string

const (
	jsonFormat format = "json"
	txtFormat  format = "txt"
)

var (
	// logFormat is the format to use for logging.
	logFormat format
	// level of the log for logrus only
	logLevel string
	// includes the calling method as a field in the log
	logMethodTrace bool
)

func InitFlag() {
	flag.StringVar(&logLevel, "log.level", "info", "log level. Possible value: panic, fatal, error, warning, info, debug, trace")
	flag.StringVar((*string)(&logFormat), "log.format", "text", "log format. Possible value: text, json")
	flag.BoolVar(&logMethodTrace, "log.method-trace", false, "include the calling method as a field in the log. Can be useful to see immediately where the log comes from")
}

func NewBuilder() *Builder {
	return &Builder{}
}

type Builder struct {
	level       logrus.Level
	format      format
	methodTrace bool
}

func (b *Builder) Level(level string) *Builder {
	l, err := logrus.ParseLevel(level)
	if err != nil {
		logrus.Warnf("Invalid log level: %s", level)
	} else {
		b.level = l
	}
	return b
}

func (b *Builder) Format(f string) *Builder {
	if f != string(jsonFormat) && f != string(txtFormat) {
		logrus.Warnf("Invalid log format: %s", f)
	} else {
		b.format = format(f)
	}
	return b
}

func (b *Builder) MethodTrace(enable bool) *Builder {
	b.methodTrace = enable
	return b
}

// SetUp is configuring the global instance of logrus.
func (b *Builder) SetUp() {
	b.build()
	logrus.SetLevel(b.level)
	logrus.SetReportCaller(b.methodTrace)

	switch b.format {
	case txtFormat:
		logrus.SetFormatter(&logrus.TextFormatter{
			// Useful when you have a TTY attached.
			// Issue explained here when this field is set to false by default:
			// https://github.com/sirupsen/logrus/issues/896
			FullTimestamp: true,
		})

	case jsonFormat:
		logrus.SetFormatter(&logrus.JSONFormatter{
			// Avoid multi-line JSON logs to make it easier to parse the
			// structured logs.
			PrettyPrint: false,
		})

	default:
		logrus.Fatalf("unknown log format %q", logFormat)
	}
}

// build will fill up any missing attribute based on the flag or if the flags are not used, then it will use default value
func (b *Builder) build() {
	// Manage the log level
	if len(b.level.String()) == 0 {
		level, err := logrus.ParseLevel(logLevel)
		if err != nil {
			logrus.Warnf("Invalid log level from flag value: %s", level)
			b.level = logrus.InfoLevel
		} else {
			b.level = level
		}
	}
	// Manage the method trace
	if !b.methodTrace {
		b.methodTrace = logMethodTrace
	}
	// Manage the log format
	if len(b.format) == 0 {
		if logFormat != jsonFormat && logFormat != txtFormat {
			logrus.Warnf("Invalid log format from the flag: %s", logFormat)
			b.format = txtFormat
		} else {
			b.format = logFormat
		}
	}
}
