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
