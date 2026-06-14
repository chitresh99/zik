package store

import (
	"sync"
	"time"
)

type ConfigVersion struct {
	Value    string    `json:"value"`
	Version  int       `json:"version"`
	UpdateAt time.Time `json:"updated_at"`
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
