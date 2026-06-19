package store

import (
	"fmt"
	"sync"
	"time"
)

type ConfigVersion struct {
	Value     string    `json:"value"`
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ConfigEntry struct {
	Current ConfigVersion   `json:"current"`
	History []ConfigVersion `json:"history"`
}

type MemoryStore struct {
	mu   sync.RWMutex
	data map[string]map[string]*ConfigEntry
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		data: make(map[string]map[string]*ConfigEntry),
	}
}

// create or update a key
func (s *MemoryStore) Set(namespace, key, value string) (*ConfigVersion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data[namespace] == nil {
		s.data[namespace] = make(map[string]*ConfigEntry)
	}

	entry, exists := s.data[namespace][key]
	if !exists {
		v := ConfigVersion{Value: value, Version: 1, UpdatedAt: time.Now().UTC()}
		s.data[namespace][key] = &ConfigEntry{Current: v, History: []ConfigVersion{}}
		return &v, nil
	}

	entry.History = append(entry.History, entry.Current)
	newVersion := ConfigVersion{
		Value:     value,
		Version:   entry.Current.Version + 1,
		UpdatedAt: time.Now().UTC(),
	}
	entry.Current = newVersion
	return &entry.Current, nil
}

func (s *MemoryStore) Get(namespace, key string) (*ConfigEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ns, ok := s.data[namespace]
	if !ok {
		return nil, fmt.Errorf("namespace %q not found", namespace)
	}
	entry, ok := ns[key]
	if !ok {
		return nil, fmt.Errorf("key %q not found in namespace %q", key, namespace)
	}

	result := *entry
	return &result, nil
}

func (s *MemoryStore) Delete(namespace, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ns, ok := s.data[namespace]
	if !ok {
		return fmt.Errorf("namespace %q not found", namespace)
	}

	if _, ok := ns[key]; !ok {
		return fmt.Errorf("key %q not found in namespace %q", key, namespace)
	}

	delete(ns, key)
	return nil
}

func (s *MemoryStore) List(namespace string) (map[string]ConfigVersion, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ns, ok := s.data[namespace]
	if !ok {
		return nil, fmt.Errorf("namespace %q not found", namespace)
	}

	result := make(map[string]ConfigVersion, len(ns))
	for k, entry := range ns {
		result[k] = entry.Current
	}
	return result, nil
}

func (s *MemoryStore) Rollback(namespace, key string) (*ConfigVersion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ns, ok := s.data[namespace]
	if !ok {
		return nil, fmt.Errorf("namespace %q not found", namespace)
	}

	entry, ok := ns[key]
	if !ok {
		return nil, fmt.Errorf("key %q not found in namespace %q", key, namespace)
	}

	if len(entry.History) == 0 {
		return nil, fmt.Errorf("no previous version to roll back to for key %q", key)
	}

	last := len(entry.History) - 1
	prev := entry.History[last]
	entry.History = entry.History[:last]
	entry.Current = prev
	return &entry.Current, nil

}
