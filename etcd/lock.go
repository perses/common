package etcd

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"go.etcd.io/etcd/clientv3"
	"go.etcd.io/etcd/clientv3/concurrency"
)

// KeyLocker is the interface that will provide methods to lock and unlock for a specific key
// Recommended usage :
// ```
//    err := k.Lock(key)
//    defer k.Unlock(key)
//    if err != nil {
//      // do something with the error
//    }
// ```
type KeyLocker interface {
	// Lock is creating a lock for the given key
	Lock(key string) error
	// Unlock is removing the lock for the given key
	Unlock(key string)
}

type keyLockerImpl struct {
	requestTimeout time.Duration
	client         *clientv3.Client
	session        *concurrency.Session
	mutex          *concurrency.Mutex
	ctx            context.Context
	cancel         context.CancelFunc
}

func newKeyLocker(requestTimeout time.Duration, client *clientv3.Client) KeyLocker {
	return &keyLockerImpl{
		client:         client,
		requestTimeout: requestTimeout,
	}
}

func (k *keyLockerImpl) Lock(key string) error {
	k.ctx, k.cancel = context.WithTimeout(context.Background(), k.requestTimeout)
	// create a concurrent session to acquire a lock on the above key
	session, err := concurrency.NewSession(k.client, concurrency.WithContext(k.ctx))
	if err != nil {
		logrus.WithError(err).Error("unable to create an etcd session")
		return err
	}

	k.session = session

	// Acquire the lock on the key
	// it's not required to have a retry logic for this part,
	// there is multiple different instance that will try to update and at the end there is a ticker that will retry anyway
	mutex := concurrency.NewMutex(session, key)
	if err := mutex.Lock(k.ctx); err != nil {
		logrus.WithError(err).Errorf("unable to acquire the lock for the key '%s'", key)
		return err
	}
	k.mutex = mutex
	return nil
}

func (k *keyLockerImpl) Unlock(key string) {
	defer k.cancel()
	if k.mutex != nil {
		if err := k.mutex.Unlock(k.ctx); err != nil {
			logrus.WithError(err).Errorf("unable to unlock the key '%s'", key)
		}
	}
	if k.session != nil {
		if err := k.session.Close(); err != nil {
			logrus.WithError(err).Errorf("unable to close the session associated to the key '%s'", key)
		}
	}
}
