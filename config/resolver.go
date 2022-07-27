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
	"github.com/fsnotify/fsnotify"
	"github.com/sirupsen/logrus"
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

type Resolver interface {
	SetEnvPrefix(prefix string) Resolver
	SetConfigFile(filename string, watch bool) Resolver
	Resolve(config interface{}) Validator
}

type configResolver struct {
	Resolver
	prefix          string
	configFile      string
	watchConfigFile bool
}

func NewResolver() Resolver {
	return &configResolver{}
}

func (c *configResolver) SetEnvPrefix(prefix string) Resolver {
	c.prefix = prefix
	return c
}

// SetConfigFile is the way to set the path to the configFile (including the name of the file)
func (c *configResolver) SetConfigFile(filename string, watch bool) Resolver {
	c.configFile = filename
	c.watchConfigFile = watch
	return c
}

func (c *configResolver) Resolve(config interface{}) Validator {
	err := c.readFromFile(config)
	if err == nil {
		err = lamenv.Unmarshal(config, []string{c.prefix})
		if c.watchConfigFile {
			err = c.watchFile(func() {
				err := c.readFromFile(config)
				if err != nil {
					logrus.Errorln("Cannot parse the watched config file:", err)
					return
				}
				logrus.Infoln("Config file reloaded.")
			})
		}
	}
	return &validatorImpl{
		err:    err,
		config: config,
	}
}

func (c *configResolver) readFromFile(config interface{}) error {
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

func (c *configResolver) watchFile(onChange func()) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if ok && event.Op&fsnotify.Write == fsnotify.Write {
					onChange()
				}
			case err := <-watcher.Errors:
				if err != nil {
					logrus.Errorln("Error with the config watching: ", err)
				}
			}
		}
	}()
	watcher.Add(c.configFile)
	return err
}
