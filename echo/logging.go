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
	"net"
	"runtime"
	"slices"
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
	entry := logrus.WithContext(ctx).WithTime(record.Time).WithFields(fields)
	// When logrus reports the caller, it would report this method, so the caller of slog is given instead.
	// It is only set in this case, as logrus writes the caller as soon as it is set.
	if record.PC != 0 && logrus.StandardLogger().ReportCaller {
		frame, _ := runtime.CallersFrames([]uintptr{record.PC}).Next()
		entry.Caller = &frame
	}
	entry.Log(logrusLevel(record.Level), record.Message)
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
// slog has no level above Error, so the levels Fatal and Panic of logrus (for the errors stopping the process) are never returned.
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
// Most of these errors are caused by the clients (like the TLS handshake errors, or the HTTP/2 protocol errors), so they are
// logged as warnings, as any client can trigger them. Only the defects of the server (see httpServerDefects) are logged as errors.
type httpServerErrorWriter struct{}

// httpServerDefects are the beginnings of the errors logged by the http.Server (net/http and its HTTP/2 server)
// that are caused by the server itself: a handler, or the system.
var httpServerDefects = []string{
	"http: panic serving",
	"http2: panic serving",
	"http: superfluous response.WriteHeader call",
	// response.Write and response.WriteHeader on a hijacked connection.
	"http: response.Write",
	"http: invalid Content-Length",
	"http: WriteHeader called with both Transfer-Encoding",
	"ignoring invalid trailer",
	"http: Accept error",
	"http: error reading directory",
}

func (httpServerErrorWriter) Write(p []byte) (int, error) {
	message := strings.TrimSuffix(string(p), "\n")
	level := logrus.WarnLevel
	if slices.ContainsFunc(httpServerDefects, func(prefix string) bool { return strings.HasPrefix(message, prefix) }) {
		level = logrus.ErrorLevel
	}
	logrus.StandardLogger().Log(level, message)
	return len(p), nil
}

// logListenerAddr logs the address listened by the server, unless it is hidden with the flag --web.hide-port.
func (s *server) logListenerAddr(addr net.Addr) {
	if hidePort {
		return
	}
	scheme := "http"
	if s.cert != "" {
		scheme = "https"
	}
	logrus.Infof("%s server started on %s", scheme, addr)
}
