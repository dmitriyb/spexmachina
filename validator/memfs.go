package validator

import (
	"bytes"
	"io/fs"
	"time"
)

// MemFS is a spec tree held entirely in memory: file contents keyed by
// slash-separated path relative to the spec root ("project.json",
// "alpha/module.json", "alpha/arch_comp1.md"), exactly the paths the same
// tree would use rooted under os.DirFS. It is the "loaded spec" the
// checkers in this package (and merkle.BuildTreeFS/CheckCompletenessFS)
// accept in place of a directory on disk — the after-state a worker has
// already applied its change to, never written until the caller accepts it
// (spec/author/arch_obligation_reporter.md, "The checkers run over an
// in-memory tree rather than a directory").
type MemFS map[string][]byte

// ReadFile implements fs.ReadFileFS, the fast path fs.ReadFile prefers: a
// lookup and a byte-slice copy, never Open/Read/Close.
func (m MemFS) ReadFile(name string) ([]byte, error) {
	data, ok := m[name]
	if !ok {
		return nil, &fs.PathError{Op: "readfile", Path: name, Err: fs.ErrNotExist}
	}
	out := make([]byte, len(data))
	copy(out, data)
	return out, nil
}

// Open implements fs.FS for any caller that does not go through
// fs.ReadFile's fast path.
func (m MemFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	data, ok := m[name]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return &memFile{name: name, r: bytes.NewReader(data), size: int64(len(data))}, nil
}

// memFile is the fs.File Open returns: a read-only view over one MemFS
// entry's bytes.
type memFile struct {
	name string
	r    *bytes.Reader
	size int64
}

func (f *memFile) Stat() (fs.FileInfo, error) { return memFileInfo{f.name, f.size}, nil }
func (f *memFile) Read(p []byte) (int, error)  { return f.r.Read(p) }
func (f *memFile) Close() error                { return nil }

// memFileInfo is the fs.FileInfo Stat returns for a memFile: a plain leaf
// file, mode 0644, mod time zero (MemFS carries no timestamp of its own).
type memFileInfo struct {
	name string
	size int64
}

func (i memFileInfo) Name() string       { return i.name }
func (i memFileInfo) Size() int64        { return i.size }
func (i memFileInfo) Mode() fs.FileMode  { return 0644 }
func (i memFileInfo) ModTime() time.Time { return time.Time{} }
func (i memFileInfo) IsDir() bool        { return false }
func (i memFileInfo) Sys() any           { return nil }
