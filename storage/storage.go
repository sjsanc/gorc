package storage

import "fmt"

type StorageType string

const (
	StorageInMemory StorageType = "memory"
)

type Store[T any] interface {
	Put(key string, value *T) error
	Get(key string) (*T, error)
	List() ([]*T, error)
	Count() (int, error)
	Delete(key string) error
}

func NewStore[T any](storageType StorageType) Store[T] {
	switch storageType {
	case StorageInMemory:
		return newMemoryStore[T]()
	default:
		panic("Unknown store type")
	}
}

func ParseStorageType(s string) (StorageType, error) {
	switch s {
	case "memory":
		return StorageInMemory, nil
	default:
		return "", fmt.Errorf("unknown storage type: %s", s)
	}
}
