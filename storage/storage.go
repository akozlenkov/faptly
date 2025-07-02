package storage

import (
	"io"
)

type Storage interface {
	Exists(path string) bool
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte) error
	WriteFileWithReader(path string, data []byte, progress io.Reader) error
	Remove(name string) error
	RemoveAll(path string) error
	Walk(root string, fn func(path string, err error) error) error
}
