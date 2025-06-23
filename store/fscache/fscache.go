// Package fscache implements a file system-based cache backend.
//
// Entries are stored as files in a directory under the user's OS cache directory by default.
// The cache location and behavior are configured via DSN query parameters.
//
// Supported DSN query parameters:
//   - appname (required): Subdirectory name for cache isolation.
//   - timeout (optional): Operation timeout (e.g., "2s", "500ms"). Default: 5m.
//
// Example DSNs:
//
//	fscache://?appname=myapp
//	fscache:///tmp/cache?appname=myapp&timeout=2s
package fscache

import (
	"cmp"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bartventer/httpcache/store"
	"github.com/bartventer/httpcache/store/driver"
	"github.com/bartventer/httpcache/store/expapi"
)

const Scheme = "fscache" // url scheme for the file system cache

//nolint:gochecknoinits // We use init to register the driver.
func init() {
	store.Register(Scheme, driver.DriverFunc(func(u *url.URL) (driver.Conn, error) {
		return Open(u)
	}))
}

var (
	ErrUserCacheDir   = errors.New("fscache: could not determine user cache dir")
	ErrMissingAppName = errors.New("fscache: appname query parameter is required")
	ErrCreateCacheDir = errors.New("fscache: could not create cache dir")
)

type Error struct {
	Op  string // operation being performed (e.g., "Get", "Set")
	Key string // optional key for which the operation failed
	Err error  // underlying error
}

func (e *Error) Error() string {
	if e.Key != "" {
		return fmt.Sprintf("fscache: %s for key %q: %v", e.Op, e.Key, e.Err)
	}
	return fmt.Sprintf("fscache: %s: %v", e.Op, e.Err)
}

func (e *Error) Unwrap() error {
	return e.Err
}

type fsCache struct {
	root    *os.Root
	fn      fileNamer     // generates file names from keys
	fnk     fileNameKeyer // extracts keys from file names
	dw      dirWalker     // used for directory walking
	timeout time.Duration // optional timeout for operations
}

const defaultTimeout = 5 * time.Minute

func parseTimeout(v string) time.Duration {
	if v == "" {
		return defaultTimeout
	}
	timeout, err := time.ParseDuration(v)
	if err != nil {
		return defaultTimeout
	}
	return cmp.Or(max(timeout, 0), defaultTimeout)
}

// Open creates a new file system cache from the provided URL.
//
// The URL path sets the base directory (defaults to the user's OS cache dir if empty).
// See the package documentation for supported DSN query parameters and examples.
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
		root:    root,
		fn:      fileNamerFunc(safeFileName),
		fnk:     fileNameKeyerFunc(keyFromFileName),
		dw:      dirWalkerFunc(filepath.WalkDir),
		timeout: parseTimeout(u.Query().Get("timeout")),
	}, nil
}

type (
	fileNamer     interface{ FileName(key string) string }
	fileNameKeyer interface{ KeyFromFileName(name string) string }
	dirWalker     interface {
		WalkDir(root string, fn fs.WalkDirFunc) error
	}
)

type (
	fileNamerFunc     func(key string) string
	fileNameKeyerFunc func(name string) string
	dirWalkerFunc     func(root string, fn fs.WalkDirFunc) error
)

func (f fileNamerFunc) FileName(key string) string                   { return f(key) }
func (f fileNameKeyerFunc) KeyFromFileName(name string) string       { return f(name) }
func (f dirWalkerFunc) WalkDir(root string, fn fs.WalkDirFunc) error { return f(root, fn) }

func safeFileName(key string) string { return base64.RawURLEncoding.EncodeToString([]byte(key)) }

func keyFromFileName(name string) string {
	data, _ := base64.RawURLEncoding.DecodeString(name)
	return string(data)
}

var _ driver.Conn = (*fsCache)(nil)
var _ expapi.KeyLister = (*fsCache)(nil)

func (c *fsCache) Get(key string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	type result struct {
		data []byte
		err  error
	}
	errc := make(chan result, 1)
	go func() {
		defer close(errc)
		data, err := c.get(key)
		if err != nil {
			errc <- result{nil, &Error{"Get", key, err}}
			return
		}
		errc <- result{data, nil}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-errc:
		return res.data, res.err
	}
}

func (c *fsCache) get(key string) ([]byte, error) {
	f, err := c.root.Open(c.fn.FileName(key))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errors.Join(driver.ErrNotExist, err)
		}
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func (c *fsCache) Set(key string, entry []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	errc := make(chan error, 1)
	go func() {
		defer close(errc)
		err := c.set(key, entry)
		if err != nil {
			errc <- &Error{"Set", key, err}
			return
		}
		errc <- nil
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errc:
		return err
	}
}

func (c *fsCache) set(key string, entry []byte) error {
	f, err := c.root.Create(c.fn.FileName(key))
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(entry)
	if err != nil {
		return err
	}
	return f.Sync()
}

func (c *fsCache) Delete(key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	errc := make(chan error, 1)
	go func() {
		defer close(errc)
		err := c.delete(key)
		if err != nil {
			errc <- &Error{"Delete", key, err}
			return
		}
		errc <- nil
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errc:
		return err
	}
}

func (c *fsCache) delete(key string) error {
	err := c.root.Remove(c.fn.FileName(key))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			err = errors.Join(driver.ErrNotExist, err)
		}
		return err
	}
	return nil
}

func (c *fsCache) Keys(prefix string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	type result struct {
		keys []string
		err  error
	}
	errc := make(chan result, 1)
	go func() {
		defer close(errc)
		keys, err := c.keys(prefix)
		if err != nil {
			errc <- result{nil, &Error{Op: "Keys", Err: fmt.Errorf("prefix %q: %w", prefix, err)}}
			return
		}
		errc <- result{keys, nil}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-errc:
		return res.keys, res.err
	}
}

func (c *fsCache) keys(prefix string) ([]string, error) {
	dirname := c.root.Name()
	keys := make([]string, 0, 10)
	err := c.dw.WalkDir(dirname, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if key := c.fnk.KeyFromFileName(filepath.Base(path)); strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return keys, nil
}
