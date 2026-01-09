package storage

type memoryStore[T any] struct {
	db map[string]*T
}

func newMemoryStore[T any]() *memoryStore[T] {
	return &memoryStore[T]{
		db: make(map[string]*T),
	}
}

func (s *memoryStore[T]) Put(key string, value *T) error {
	s.db[key] = value
	return nil
}

func (s *memoryStore[T]) Get(key string) (*T, error) {
	value, _ := s.db[key]
	return value, nil
}

func (s *memoryStore[T]) List() ([]*T, error) {
	var list []*T
	for _, value := range s.db {
		list = append(list, value)
	}
	return list, nil
}

func (s *memoryStore[T]) Count() (int, error) {
	return len(s.db), nil
}
