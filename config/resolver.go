package config

import (
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
	SetConfigFile(filename string) Resolver
	Resolve(config interface{}) Validator
}

type configResolver struct {
	Resolver
	prefix     string
	configFile string
}

func NewResolver() Resolver {
	return &configResolver{}
}

func (c *configResolver) SetEnvPrefix(prefix string) Resolver {
	c.prefix = prefix
	return c
}

// SetConfigFile is the way to set the path to the configFile (including the name of the file)
func (c *configResolver) SetConfigFile(filename string) Resolver {
	c.configFile = filename
	return c
}

func (c *configResolver) Resolve(config interface{}) Validator {
	err := c.readFromFile(config)
	if err == nil {
		err = lamenv.New().Unmarshal(config, []string{c.prefix})
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
