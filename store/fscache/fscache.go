// Package fscache implements a file system-based store.Cache.
//
// Entries are stored as files in a directory under the user's OS cache directory by default.
// The cache location is configured via URL; see [Open] for details.
//
// Users should call [fsCache.Close] on the cache when done to release resources promptly.
package fscache

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/bartventer/httpcache/store"
)

const Scheme = "fscache" // url scheme for the file system cache

//nolint:gochecknoinits // We use init to register the driver.
func init() {
	store.Register(Scheme, store.DriverFunc(func(u *url.URL) (store.Cache, error) {
		return Open(u)
	}))
}

type fsCache struct {
	root *os.Root
	fn   fileNamer // generates file names from keys, useful for testing purposes
}

var (
	ErrUserCacheDir   = errors.New("fscache: could not determine user cache dir")
	ErrMissingAppName = errors.New("fscache: appname query parameter is required")
	ErrCreateCacheDir = errors.New("fscache: could not create cache dir")
)

// Open creates a new file system cache from the provided URL.
//
// The URL path sets the base directory (defaults to the user's OS cache dir if empty).
// The "appname" query parameter is required and specifies a subdirectory.
//
// Examples:
//
//	fscache://?appname=myapp           → <user cache dir>/myapp
//	fscache:///tmp/cache?appname=myapp → /tmp/cache/myapp
func Open(u *url.URL) (*fsCache, error) {
	var base string
	if u.Path != "" && u.Path != "/" {
		base = u.Path
	} else {
		userCacheDir, err := os.UserCacheDir()
		if err != nil {
			return nil, errors.Join(ErrUserCacheDir, err)
		}
		base = userCacheDir
	}

	appname := u.Query().Get("appname")
	if appname == "" {
		return nil, ErrMissingAppName
	}
	base = filepath.Join(base, appname)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return nil, errors.Join(ErrCreateCacheDir, err)
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		return nil, fmt.Errorf("fscache: could not open cache directory %q: %w", base, err)
	}
	return &fsCache{
		root: root,
		fn:   fileNamerFunc(safeFileName),
	}, nil
}

func safeFileName(key string) string {
	return strings.ReplaceAll(key, string(os.PathSeparator), "_")
}

type fileNamer interface {
	FileName(key string) string
}

type fileNamerFunc func(key string) string

func (f fileNamerFunc) FileName(key string) string {
	return f(key)
}

var _ store.Cache = (*fsCache)(nil)

func (c *fsCache) Get(key string) ([]byte, error) {
	f, err := c.root.Open(c.fn.FileName(key))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			err = errors.Join(
				store.ErrNotExist,
				fmt.Errorf("fscache: entry for key %q does not exist: %w", key, err),
			)
		}
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func (c *fsCache) Set(key string, entry []byte) error {
	f, err := c.root.Create(c.fn.FileName(key))
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(entry)
	return err
}

func (c *fsCache) Delete(key string) error {
	err := c.root.Remove(c.fn.FileName(key))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			err = errors.Join(
				store.ErrNotExist,
				fmt.Errorf("fscache: entry for key %q does not exist: %w", key, err),
			)
		}
		return err
	}
	return nil
}

// Close releases resources held by the cache.
//
// Users should always call Close when finished with the cache to promptly release
// OS resources. If Close is not called, resources will eventually be released by
// a finalizer, but this may happen much later.
func (c *fsCache) Close() error {
	return c.root.Close()
}
