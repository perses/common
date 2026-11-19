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
	"io"
	"testing"

	"github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
)

// The values of the flags before InitFlag is called by TestBuild, like in an application that doesn't register them.
var (
	logLevelWithoutFlags       = logLevel
	logFormatWithoutFlags      = logFormat
	logMethodTraceWithoutFlags = logMethodTrace
)

// newLogHook captures the logs of the standard logrus logger until the end of the test.
func newLogHook(t *testing.T) *logrustest.Hook {
	t.Helper()
	logger := logrus.StandardLogger()
	origHooks := logger.ReplaceHooks(make(logrus.LevelHooks))
	origOutput := logger.Out
	logger.SetOutput(io.Discard)
	t.Cleanup(func() {
		logger.ReplaceHooks(origHooks)
		logger.SetOutput(origOutput)
	})
	return logrustest.NewLocal(logger)
}

// logMessages returns the messages of the logs captured by the hook.
func logMessages(hook *logrustest.Hook) []string {
	var messages []string
	for _, entry := range hook.AllEntries() {
		messages = append(messages, entry.Message)
	}
	return messages
}

// setFlagValues changes the values of the flags until the end of the test.
func setFlagValues(t *testing.T, level string, f format, methodTrace bool) {
	t.Helper()
	origLevel, origFormat, origMethodTrace := logLevel, logFormat, logMethodTrace
	logLevel, logFormat, logMethodTrace = level, f, methodTrace
	t.Cleanup(func() {
		logLevel, logFormat, logMethodTrace = origLevel, origFormat, origMethodTrace
	})
}

func TestBuild(t *testing.T) {
	InitFlag()
	infoLevel := logrus.InfoLevel
	debugLevel := logrus.DebugLevel
	testCase := []struct {
		name            string
		flagLevel       string
		flagFormat      string
		flagMethodTrace string
		result          *Builder
	}{
		{
			name: "default",
			result: &Builder{
				level:       &infoLevel,
				format:      txtFormat,
				methodTrace: false,
			},
		},
		{
			name:            "set flag",
			flagLevel:       "debug",
			flagFormat:      "json",
			flagMethodTrace: "true",
			result: &Builder{
				level:       &debugLevel,
				format:      jsonFormat,
				methodTrace: true,
			},
		},
	}
	for _, test := range testCase {
		t.Run(test.name, func(t *testing.T) {
			flag.Parse()
			if len(test.flagLevel) > 0 {
				assert.NoError(t, flag.Set("log.level", test.flagLevel))
			}
			if len(test.flagFormat) > 0 {
				assert.NoError(t, flag.Set("log.format", test.flagFormat))
			}
			if len(test.flagMethodTrace) > 0 {
				assert.NoError(t, flag.Set("log.method-trace", test.flagMethodTrace))
			}
			b := NewBuilder()
			b.build()
			assert.Equal(t, test.result, b)
		})
	}
}

func TestBuild_WithoutFlags(t *testing.T) {
	setFlagValues(t, logLevelWithoutFlags, logFormatWithoutFlags, logMethodTraceWithoutFlags)
	hook := newLogHook(t)
	infoLevel := logrus.InfoLevel
	debugLevel := logrus.DebugLevel
	testCase := []struct {
		name    string
		builder *Builder
		result  *Builder
	}{
		{
			name:    "nothing set",
			builder: NewBuilder(),
			result: &Builder{
				level:  &infoLevel,
				format: txtFormat,
			},
		},
		{
			name:    "level set",
			builder: NewBuilder().Level("debug"),
			result: &Builder{
				level:  &debugLevel,
				format: txtFormat,
			},
		},
		{
			name:    "format set",
			builder: NewBuilder().Format("json"),
			result: &Builder{
				level:  &infoLevel,
				format: jsonFormat,
			},
		},
	}
	for _, test := range testCase {
		t.Run(test.name, func(t *testing.T) {
			hook.Reset()
			test.builder.build()
			assert.Equal(t, test.result, test.builder)
			// The default values of the flags are used, so there is nothing to warn about.
			assert.Empty(t, logMessages(hook))
		})
	}
}

func TestBuild_InvalidFlagValues(t *testing.T) {
	setFlagValues(t, "verbose", "xml", false)
	hook := newLogHook(t)
	b := NewBuilder()
	b.build()
	infoLevel := logrus.InfoLevel
	assert.Equal(t, &Builder{level: &infoLevel, format: txtFormat}, b)
	// The warnings must tell which values are invalid.
	assert.Equal(t, []string{
		"Invalid log level from flag value: verbose",
		"Invalid log format from the flag: xml",
	}, logMessages(hook))
	for _, entry := range hook.AllEntries() {
		assert.Equal(t, logrus.WarnLevel, entry.Level)
	}
}
