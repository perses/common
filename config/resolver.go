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

// Package config provides a single way to manage the configuration of your application.
// The configuration can be a yaml file and/or a list of environment variable.
// To set the config using the environment, this package is using the package github.com/nexucis/lamenv,
// which is able to determinate what is the environment variable that matched the different attribute tof the struct.
// By default it is based on the yaml tag provided.
//
// The main entry point of this package is the struct Resolver.
// This struct will allow you to set the path to your config file if you have one and to give the prefix of all of your environment variable.
// Note:
//   1. A good practice is to prefix your environment variable by the name of your application.
//   2. The config file is not mandatory, you can manage all you configuration using the environment variable.
//   3. The config by environment is always overriding the config by file.
//
// The Resolver at the end returns an object that implements the interface Validator.
// Each config/struct can implement this interface in order to provide a single way to verify the configuration and to set the default value.
// The object returned by the Resolver will loop other different structs that are parts of the config and execute the method Verify if implemented.
//
// Example:
//   import (
//           "fmt"
//
//           "github.com/perses/common/config"
//   )
//
//    type Config struct {
//	    Etcd *EtcdConfig `yaml:"etcd"`
//    }
//
//    func (c *Config) Verify() error {
//      if c.EtcdConfig == nil {
//        return fmt.Errorf("etcd config cannot be empty")
//      }
//    }
//
//    func Resolve(configFile string) (Config, error) {
//	    c := Config{}
//	    return c, config.NewResolver().
//		  SetConfigFile(configFile).
//		  SetEnvPrefix("PERSES").
//		  Resolve(&c).
//		  Verify()
//    }
package config

import (
	"crypto/sha1"
	"github.com/perses/common/osutil"
	"github.com/sirupsen/logrus"
	"hash"
	"io/ioutil"
	"os"
	"reflect"

	"github.com/nexucis/lamenv"
	"gopkg.in/yaml.v2"
)

type Validator interface {
	Verify() error
}

type validatorImpl struct {
	Validator
	err    error
	config interface{}
}

// Verify will check if the different attribute of the config is implementing the interface Validator.
// If it's the case, then it will call the method Verify of each attribute.
func (v *validatorImpl) Verify() error {
	if v.err != nil {
		return v.err
	}
	ifv := reflect.ValueOf(v.config)
	return verifyRec(ifv)
}

func checkPointer(ptr reflect.Value) error {
	if ptr.IsNil() {
		return nil
	}
	if p, ok := ptr.Interface().(Validator); ok {
		if err := p.Verify(); err != nil {
			return err
		}
	}
	return nil
}

func verifyRec(conf reflect.Value) error {
	v := conf
	if conf.Kind() != reflect.Ptr {
		// that means it's not a pointer, so we have to create one to be able to then know if it implements the interface Validator
		ptr := reflect.New(v.Type())
		ptr.Elem().Set(v)
		// so now we are able to check if the pointer is implementing the interface
		if err := checkPointer(ptr); err != nil {
			return err
		}
		// in case the method Verify() is setting some parameter in the struct, we have to save these changes
		v.Set(ptr.Elem())
	} else {
		if err := checkPointer(v); err != nil {
			return err
		}
		// for what is coming next, if it's a pointer, we need to access to the value itself
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			if err := verifyRec(v.Index(i)); err != nil {
				return err
			}
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			attr := v.Field(i)
			if len(v.Type().Field(i).PkgPath) > 0 {
				// the field is not exported, so no need to look at it as we won't be able to set it in a later stage
				continue
			}
			if err := verifyRec(attr); err != nil {
				return err
			}
		}
	}

	return nil
}

type Resolver[TConfig any] interface {
	SetEnvPrefix(prefix string) Resolver[TConfig]
	SetConfigFile(filename string) Resolver[TConfig]
	AddChangeCallback(func(*TConfig)) Resolver[TConfig]
	Resolve(config *TConfig) Validator
}

type configResolver[TConfig any] struct {
	Resolver[TConfig]
	prefix         string
	configFile     string
	watchCallbacks []func(*TConfig)
}

func NewResolver[TConfig any]() Resolver[TConfig] {
	return &configResolver[TConfig]{}
}

func (c *configResolver[TConfig]) SetEnvPrefix(prefix string) Resolver[TConfig] {
	c.prefix = prefix
	return c
}

// SetConfigFile is the way to set the path to the configFile (including the name of the file)
func (c *configResolver[TConfig]) SetConfigFile(filename string) Resolver[TConfig] {
	c.configFile = filename
	return c
}

func (c *configResolver[TConfig]) AddChangeCallback(callback func(*TConfig)) Resolver[TConfig] {
	c.watchCallbacks = append(c.watchCallbacks, callback)
	return c
}

func (c *configResolver[TConfig]) Resolve(config *TConfig) Validator {
	err := c.readFromFile(config)
	if err == nil {
		err = lamenv.Unmarshal(config, []string{c.prefix})
		if len(c.watchCallbacks) != 0 {
			c.watchFile(config)
		}
	}
	return &validatorImpl{
		err:    err,
		config: config,
	}
}

func (c *configResolver[TConfig]) watchFile(config *TConfig) {
	previousHash := c.hashConfig(config)

	osutil.WatchFile(c.configFile, func() {
		err := c.readFromFile(config)
		if err != nil {
			logrus.Errorf("Cannot parse the watched config file %s: %s", c.configFile, err)
			return
		}

		newHash := c.hashConfig(config)
		if reflect.DeepEqual(newHash, previousHash) {
			return
		}
		previousHash = newHash

		for _, c := range c.watchCallbacks {
			c(config)
		}
	})
}

func (c *configResolver[TConfig]) readFromFile(config *TConfig) error {
	if len(c.configFile) == 0 {
		return nil
	}
	if _, err := os.Stat(c.configFile); err == nil {
		// the file exists so we should unmarshal the configuration using yaml
		data, fileErr := ioutil.ReadFile(c.configFile)
		if fileErr != nil {
			return fileErr
		}
		if unmarshalErr := yaml.UnmarshalStrict(data, config); unmarshalErr != nil {
			return unmarshalErr
		}
	} else {
		return err
	}
	return nil
}

func (c *configResolver[TConfig]) hashConfig(config *TConfig) hash.Hash {
	hash := sha1.New()
	data, err := yaml.Marshal(config)
	if err != nil {
		logrus.Errorf("Cannot marshal the config: %s", err)
		return nil
	}
	_, err = hash.Write(data)
	if err != nil {
		logrus.Errorf("Cannot compute the hash of the configuration: %s", err)
		return nil
	}
	return hash
}
