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

package etcd

import (
	"fmt"
	"time"

	"github.com/perses/common/config"
	"go.etcd.io/etcd/clientv3"
	"google.golang.org/grpc"
)

func NewETCDClient(conf config.EtcdConfig) (*clientv3.Client, error) {
	timeout := time.Duration(conf.RequestTimeoutSeconds) * time.Second
	etcdClient, err := clientv3.New(clientv3.Config{
		Endpoints:        conf.BuildEndpoints(),
		AutoSyncInterval: 0,
		DialTimeout:      timeout,
		DialOptions:      []grpc.DialOption{grpc.WithBlock()},
		Username:         conf.User,
		Password:         conf.Password,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to initialize the connection to etcd: %w", err)
	}
	return etcdClient, nil
}
