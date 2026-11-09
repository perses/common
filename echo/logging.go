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

package echo

import (
	"context"
	"log/slog"
	"maps"
	"strings"

	"github.com/sirupsen/logrus"
)

// logrusHandler is a slog.Handler writing the records with the standard logger of logrus.
// echo v5 logs with log/slog, so this handler is given to echo to have its logs with the same format, level and output as the other logs.
// The attributes are written as logrus fields. The keys of the attributes in a group are prefixed by the name of the group (like "group.key").
type logrusHandler struct {
	// fields are the attributes added by WithAttrs.
	fields logrus.Fields
	// groupPrefix prefixes the keys of the next attributes. It is built from the groups added by WithGroup.
	groupPrefix string
}

func (h logrusHandler) Enabled(_ context.Context, level slog.Level) bool {
	return logrus.IsLevelEnabled(logrusLevel(level))
}

func (h logrusHandler) Handle(ctx context.Context, record slog.Record) error {
	fields := make(logrus.Fields, len(h.fields)+record.NumAttrs())
	maps.Copy(fields, h.fields)
	record.Attrs(func(attr slog.Attr) bool {
		addField(fields, h.groupPrefix, attr)
		return true
	})
	// logrus uses the current time when the time of the record is zero.
	logrus.WithContext(ctx).WithTime(record.Time).WithFields(fields).Log(logrusLevel(record.Level), record.Message)
	return nil
}

func (h logrusHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	fields := make(logrus.Fields, len(h.fields)+len(attrs))
	maps.Copy(fields, h.fields)
	for _, attr := range attrs {
		addField(fields, h.groupPrefix, attr)
	}
	return logrusHandler{fields: fields, groupPrefix: h.groupPrefix}
}

func (h logrusHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	return logrusHandler{fields: h.fields, groupPrefix: h.groupPrefix + name + "."}
}

// addField adds the attribute to the fields. The attributes of a group are added one by one, prefixed by the name of the group.
func addField(fields logrus.Fields, prefix string, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	if attr.Value.Kind() == slog.KindGroup {
		// Like the handlers of slog, the attributes of a group without key are inlined.
		if attr.Key != "" {
			prefix += attr.Key + "."
		}
		for _, groupAttr := range attr.Value.Group() {
			addField(fields, prefix, groupAttr)
		}
		return
	}
	// An attribute without key (like the empty attributes, ignored by the handlers of slog) cannot be a logrus field.
	if attr.Key == "" {
		return
	}
	fields[prefix+attr.Key] = attr.Value.Any()
}

// logrusLevel converts a slog level to the closest logrus level.
// The levels Fatal and Panic are never returned, as logrus would exit or panic after logging.
func logrusLevel(level slog.Level) logrus.Level {
	switch {
	case level >= slog.LevelError:
		return logrus.ErrorLevel
	case level >= slog.LevelWarn:
		return logrus.WarnLevel
	case level >= slog.LevelInfo:
		return logrus.InfoLevel
	case level >= slog.LevelDebug:
		return logrus.DebugLevel
	default:
		return logrus.TraceLevel
	}
}

// httpServerErrorWriter writes with logrus every line logged by the http.Server.
// The TLS handshake errors are caused by the clients (like a load balancer only checking that the port is open, or a client
// not supporting the TLS versions or cipher suites configured), so they are logged as warnings.
// The other errors (like a panic not recovered by a middleware) are logged as errors.
type httpServerErrorWriter struct{}

func (httpServerErrorWriter) Write(p []byte) (int, error) {
	message := strings.TrimSuffix(string(p), "\n")
	level := logrus.ErrorLevel
	if strings.HasPrefix(message, "http: TLS handshake error") {
		level = logrus.WarnLevel
	}
	logrus.StandardLogger().Log(level, message)
	return len(p), nil
}
