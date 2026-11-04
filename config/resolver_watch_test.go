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

package config

import (
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
	logrusTest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWatchConfigFile_LogTheWatchError(t *testing.T) {
	hook := logrusTest.NewGlobal()
	defer logrus.StandardLogger().ReplaceHooks(make(logrus.LevelHooks))

	type watchConfig struct {
		Field string `yaml:"field"`
	}
	resolver := &configResolver[watchConfig]{
		// The parent directory doesn't exist, so it cannot be watched.
		configFile:     filepath.Join(t.TempDir(), "missing", "config.yaml"),
		watchCallbacks: []func(*watchConfig){func(*watchConfig) {}},
	}
	resolver.watchConfigFile(&watchConfig{})

	entry := hook.LastEntry()
	require.NotNil(t, entry)
	assert.Equal(t, logrus.ErrorLevel, entry.Level)
	assert.Contains(t, entry.Message, "Failed to watch the config file")
	watchErr, isError := entry.Data[logrus.ErrorKey].(error)
	require.True(t, isError, "the error returned by the watcher must be logged")
	assert.Error(t, watchErr)
}
