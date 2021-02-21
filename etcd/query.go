package etcd

// Query is the interface that all queryParameter should implement.
type Query interface {
	Build() (string, error)
}
