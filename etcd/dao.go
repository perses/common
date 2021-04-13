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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"time"

	"github.com/sirupsen/logrus"
	"go.etcd.io/etcd/api/v3/etcdserverpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func encode(entity interface{}) (string, error) {
	out, err := json.Marshal(entity)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func decode(data []byte, entity interface{}) error {
	return json.Unmarshal(data, entity)
}

// DAO defines CRUD method
type DAO interface {
	io.Closer
	Create(key string, entity interface{}) error
	Upsert(key string, entity interface{}) error
	Get(key string, entity interface{}) error
	Query(query Query, slice interface{}) error
	Delete(key string) error
	Watch(ctx context.Context, query Query) (clientv3.WatchChan, error)
	RequestLocker() KeyLocker
	HealthCheck() bool
}

// NewDAO creates a new instance of DAO interface
func NewDAO(client *clientv3.Client, timeout time.Duration) DAO {
	kv := clientv3.NewKV(client)
	return &daoImpl{
		client:         client,
		kvClient:       kv,
		watcher:        clientv3.NewWatcher(client),
		requestTimeout: timeout,
	}
}

// daoImpl is an object that implements all generic CRUD method
type daoImpl struct {
	DAO
	client         *clientv3.Client
	kvClient       clientv3.KV
	watcher        clientv3.Watcher
	requestTimeout time.Duration
}

func (d *daoImpl) Close() error {
	return d.client.Close()
}

func (d *daoImpl) Create(key string, entity interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), d.requestTimeout)
	defer cancel()
	gr, err := d.kvClient.Get(ctx, key)
	if err != nil {
		return err
	}
	if gr.Count > 0 {
		return &Error{Key: key, Code: ErrorCodeKeyConflict}
	}
	s, err := encode(entity)
	if err != nil {
		return err
	}
	_, err = d.kvClient.Put(ctx, key, s)
	if err != nil {
		return err
	}
	return nil
}

func (d *daoImpl) Get(key string, entity interface{}) error {
	ctx, cancel := context.WithTimeout(context.Background(), d.requestTimeout)
	defer cancel()
	gr, err := d.kvClient.Get(ctx, key)
	if err != nil {
		return err
	}
	if gr.Count == 0 {
		return &Error{Key: key, Code: ErrorCodeKeyNotFound}
	}
	return decode(gr.Kvs[0].Value, entity)
}

// Query returns the list of values that starts with a certain prefix
// slice must be a pointer to the slice. It will contain the result of the query if no errors are detected
func (d *daoImpl) Query(query Query, slice interface{}) error {
	typeParameter := reflect.TypeOf(slice)
	result := reflect.ValueOf(slice)
	// to avoid any miss usage when using this method, slice should be a pointer to a slice.
	// first check if slice is a pointer
	if typeParameter.Kind() != reflect.Ptr {
		return fmt.Errorf("slice in parameter is not a pointer to a slice but a '%s'", typeParameter.Kind())
	}

	// it's a pointer, so move to the actual element behind the pointer.
	// Having a pointer avoid to get the error:
	//           reflect.Value.Set using unaddressable value
	// It's because the slice is usually not initialized and doesn't have any memory allocated.
	// So it's simpler to required a pointer at the beginning.
	sliceElem := result.Elem()
	typeParameter = typeParameter.Elem()

	if typeParameter.Kind() != reflect.Slice {
		return fmt.Errorf("slice in parameter is not actually a slice but a '%s'", typeParameter.Kind())
	}
	ctx, cancel := context.WithTimeout(context.Background(), d.requestTimeout)
	defer cancel()
	q, err := query.Build()
	if err != nil {
		return fmt.Errorf("unable to build the query: %s", err)
	}
	gr, err := d.kvClient.Get(ctx, q, clientv3.WithPrefix())
	if err != nil {
		return err
	}

	if len(gr.Kvs) <= 0 {
		// in case the result is empty, let's initialize the slice just to avoid to return a nil slice
		sliceElem = reflect.MakeSlice(typeParameter, 0, 0)
	}

	for _, kv := range gr.Kvs {
		// first create a pointer with the accurate type
		var value reflect.Value
		if typeParameter.Elem().Kind() != reflect.Ptr {
			value = reflect.New(typeParameter.Elem())
		} else {
			// in case it's a pointer, then we should create a pointer of the struct and not a pointer of a pointer
			value = reflect.New(typeParameter.Elem().Elem())
		}
		// then get back the actual struct behind the value.
		obj := value.Interface()
		if err := decode(kv.Value, &obj); err != nil {
			return fmt.Errorf("error decoding the value associated with the key '%s': %w", kv.Key, err)
		}
		sliceElem.Set(reflect.Append(sliceElem, value))
	}
	// at the end reset the element of the slice to ensure we didn't disconnect the link between the pointer to the slice and the actual slice
	result.Elem().Set(sliceElem)
	return nil
}

// Upsert creates/updates a key-value pair in etcd.
func (d *daoImpl) Upsert(key string, entity interface{}) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), d.requestTimeout)
	defer cancel()
	s, err := encode(entity)
	if err != nil {
		return err
	}
	_, err = d.kvClient.Put(ctx, key, s)
	if err != nil {
		return err
	}
	return nil
}

func (d *daoImpl) Delete(key string) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), d.requestTimeout)
	defer cancel()
	gr, err := d.kvClient.Delete(ctx, key)
	if err != nil {
		return err
	}
	if gr.Deleted == 0 {
		return &Error{Key: key, Code: ErrorCodeKeyNotFound}
	}
	return nil
}

func (d *daoImpl) Watch(ctx context.Context, query Query) (clientv3.WatchChan, error) {
	q, err := query.Build()
	if err != nil {
		return nil, fmt.Errorf("unable to build the query: %s", err)
	}
	return d.watcher.Watch(ctx, q, clientv3.WithPrefix()), nil
}

func (d *daoImpl) RequestLocker() KeyLocker {
	return newKeyLocker(d.requestTimeout, d.client)
}

// HealthCheck pings etcd and return false if there is an issue
// It returns true if everything goes right.
func (d *daoImpl) HealthCheck() bool {
	ctx, cancel := context.WithTimeout(context.Background(), d.requestTimeout)
	defer cancel()
	alarmList, err := d.client.AlarmList(ctx)
	if err != nil {
		logrus.WithError(err).Error("an error occurred while trying to ping etcd")
		return false
	}
	for _, alarm := range alarmList.Alarms {
		if alarm.GetAlarm() != etcdserverpb.AlarmType_NONE {
			logrus.Errorf("Alarm raised by etcd: alarm %s", alarm.GetAlarm().String())
			return false
		}
	}
	return true
}
