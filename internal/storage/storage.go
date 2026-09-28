package storage

// StorageService defines the interface for media storage operations.
type StorageService interface {
	SaveMedia(phone string, data []byte, mime string) (string, error)
	GetMedia(fname string) ([]byte, string, error)
}

// Storage is an alias for StorageService to maintain compatibility.
type Storage = StorageService
