// Copyright 2021 Amadeus s.a.s
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

import "fmt"

const defaultETCDPort = 2379

type EtcdProtocol string

const (
	EtcdAsHTTPProtocol  EtcdProtocol = "http"
	EtcdAsHTTPSProtocol EtcdProtocol = "https"
)

// KindMap stores all the possible values of the Kind enum
var etcdpMap = map[EtcdProtocol]bool{
	EtcdAsHTTPProtocol:  true,
	EtcdAsHTTPSProtocol: true,
}

func (c *EtcdProtocol) Verify() error {
	if _, ok := etcdpMap[*c]; !ok {
		return fmt.Errorf("%s is an unvalid protocol to make the connection to etcd", *c)
	}
	return nil
}

// Connection is a configuration of an etcd host and port
type Connection struct {
	Host string `yaml:"host"`
	Port uint64 `yaml:"port,omitempty"`
}

func (c *Connection) Verify() error {
	if len(c.Host) <= 0 {
		return fmt.Errorf("host cannot be null")
	}
	return nil
}

// EtcdConfig defines the way to configure the connection to the etcd database
type EtcdConfig struct {
	Connections           []Connection `yaml:"connections"`
	Protocol              EtcdProtocol `yaml:"protocol,omitempty"`
	User                  string       `yaml:"user,omitempty"`
	Password              string       `yaml:"password,omitempty"`
	RequestTimeoutSeconds uint64       `yaml:"request_timeout"`
}

func (c *EtcdConfig) Verify() error {
	if len(c.Connections) == 0 {
		return fmt.Errorf("the connections must be specified")
	}

	if len(c.User) > 0 && len(c.Password) <= 0 {
		return fmt.Errorf("password is not set for the user %s", c.User)
	}

	if len(c.Password) > 0 && len(c.User) <= 0 {
		return fmt.Errorf("user is not set while the password is")
	}

	if c.RequestTimeoutSeconds <= 0 {
		c.RequestTimeoutSeconds = 120
	}
	for i := 0; i < len(c.Connections); i++ {
		if c.Connections[i].Port <= 0 {
			c.Connections[i].Port = defaultETCDPort
		}
	}

	if len(c.Protocol) <= 0 {
		c.Protocol = EtcdAsHTTPProtocol
	}
	return nil
}

// BuildEndpoints returns a slice of URLs that can be used in the method clientv3.New
func (c *EtcdConfig) BuildEndpoints() []string {
	var urls []string
	for _, connection := range c.Connections {
		urls = append(urls, fmt.Sprintf("%s://%s:%d", c.Protocol, connection.Host, connection.Port))
	}
	return urls
}
