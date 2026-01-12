package storage

import "sync"

type memoryStore[T any] struct {
	db map[string]*T
	mu sync.RWMutex
}

func newMemoryStore[T any]() *memoryStore[T] {
	return &memoryStore[T]{
		db: make(map[string]*T),
	}
}

func (s *memoryStore[T]) Put(key string, value *T) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.db[key] = value
	return nil
}

func (s *memoryStore[T]) Get(key string) (*T, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, _ := s.db[key]
	return value, nil
}

func (s *memoryStore[T]) List() ([]*T, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var list []*T
	for _, value := range s.db {
		list = append(list, value)
	}
	return list, nil
}

func (s *memoryStore[T]) Count() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.db), nil
}

func (s *memoryStore[T]) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.db, key)
	return nil
}
