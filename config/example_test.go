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

package config_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/perses/common/config"
)

type DatabaseConfig struct {
	URL      string `yaml:"url"`
	MaxConns int    `yaml:"max_conns"`
}

// Verify checks the config and sets the default values. It is called even if the field database is not in the config file.
func (c *DatabaseConfig) Verify() error {
	if len(c.URL) == 0 {
		return errors.New("database.url cannot be empty")
	}
	if c.MaxConns == 0 {
		c.MaxConns = 10
	}
	return nil
}

type Config struct {
	Database DatabaseConfig `yaml:"database"`
}

// Resolve reads the config file, then overrides it with the environment variables, like PERSES_DATABASE_URL.
func Resolve(configFile string) (Config, error) {
	var c Config
	err := config.NewResolver[Config]().
		SetConfigFile(configFile).
		SetEnvPrefix("PERSES").
		Resolve(&c).
		Verify()
	return c, err
}

func ExampleNewResolver() {
	dir, err := os.MkdirTemp("", "config")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = os.RemoveAll(dir) }()
	configFile := filepath.Join(dir, "config.yaml")
	if err = os.WriteFile(configFile, []byte("database:\n  url: postgres://file\n"), 0o600); err != nil {
		fmt.Println(err)
		return
	}

	c, err := Resolve(configFile)
	fmt.Println(c.Database.URL, c.Database.MaxConns, err)

	// the environment variables override the config file
	_ = os.Setenv("PERSES_DATABASE_URL", "postgres://env")
	defer func() { _ = os.Unsetenv("PERSES_DATABASE_URL") }()
	c, err = Resolve(configFile)
	fmt.Println(c.Database.URL, c.Database.MaxConns, err)
	// Output:
	// postgres://file 10 <nil>
	// postgres://env 10 <nil>
}
