package etcd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"time"

	"go.etcd.io/etcd/clientv3"
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

// returns the list of values that starts with a certain prefix
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
	result = result.Elem()
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
		result.Set(reflect.Append(result, value))
	}
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
