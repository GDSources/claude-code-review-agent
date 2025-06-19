package review

import (
	"os"
)

type DefaultFileSystemManager struct{}

func NewDefaultFileSystemManager() *DefaultFileSystemManager {
	return &DefaultFileSystemManager{}
}

func (f *DefaultFileSystemManager) CreateTempDir(prefix string) (string, error) {
	return os.MkdirTemp("", prefix)
}

func (f *DefaultFileSystemManager) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

func (f *DefaultFileSystemManager) Exists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
