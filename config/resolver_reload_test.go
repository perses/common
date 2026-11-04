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
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type reloadConfig struct {
	Secret string `yaml:"secret"`
	Level  string `yaml:"level"`
	Port   int    `yaml:"port"`
}

func (c *reloadConfig) Verify() error {
	if c.Level == "invalid" {
		return fmt.Errorf("invalid level")
	}
	if c.Port == 0 {
		c.Port = 8080
	}
	return nil
}

func TestResolveImpl_WatchConfigShouldResolveAndVerifyTheNewConfig(t *testing.T) {
	configFile := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte("secret: fromfile\nlevel: info\n"), 0600))
	t.Setenv("RELOAD_SECRET", "fromenv")

	var mutex sync.Mutex
	var received []reloadConfig
	receivedConfigs := func() []reloadConfig {
		mutex.Lock()
		defer mutex.Unlock()
		return slices.Clone(received)
	}

	var config reloadConfig
	err := NewResolver[reloadConfig]().
		SetEnvPrefix("RELOAD").
		SetConfigFile(configFile).
		AddChangeCallback(func(newConfig *reloadConfig) {
			mutex.Lock()
			defer mutex.Unlock()
			received = append(received, *newConfig)
		}).
		Resolve(&config).
		Verify()
	require.NoError(t, err)
	assert.Equal(t, reloadConfig{Secret: "fromenv", Level: "info", Port: 8080}, config)

	// The new config is resolved like the initial one: the environment variable overrides the file, and the default values are set.
	require.NoError(t, os.WriteFile(configFile, []byte("secret: fromfile\nlevel: debug\n"), 0600))
	expected := reloadConfig{Secret: "fromenv", Level: "debug", Port: 8080}
	assert.Eventually(t, func() bool {
		return slices.Contains(receivedConfigs(), expected)
	}, 5*time.Second, 10*time.Millisecond)

	// An invalid config is never passed to the callbacks.
	require.NoError(t, os.WriteFile(configFile, []byte("secret: fromfile\nlevel: invalid\n"), 0600))
	time.Sleep(500 * time.Millisecond)
	for _, c := range receivedConfigs() {
		assert.NotEqual(t, "invalid", c.Level)
		assert.Equal(t, "fromenv", c.Secret)
	}
}
