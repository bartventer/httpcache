package httpcache

// Cache describes the interface implemented by types that can store, retrieve, and delete
// cache entries.
type Cache interface {
	Get(key string) ([]byte, error)
	Set(key string, entry []byte) error
	Delete(key string) error
}
