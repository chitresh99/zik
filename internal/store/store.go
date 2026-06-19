package store

type Store interface {
	Set(namespace, key, value string) (*ConfigVersion, error)
	Get(namespace, key string) (*ConfigEntry, error)
	Delete(namespace, key string) error
	List(namespace string) (map[string]ConfigVersion, error)
	Rollback(namespace, key string) (*ConfigVersion, error)
}
