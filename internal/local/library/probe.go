package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

const maxFilesystemProbes = 4

type probeKey struct {
	path         string
	canonicalize bool
}

type probeCall struct {
	done chan struct{}
	path string
	err  error
}

// Filesystem calls cannot reliably be canceled on a stalled NAS. Retain their
// slots until they actually return, and share in-flight work for the same root.
// Callers can stop waiting without spawning more work beyond this fixed bound.
type filesystemProbe struct {
	mu      sync.Mutex
	active  map[probeKey]*probeCall
	changed chan struct{}
	inspect func(string, bool) (string, error)
}

func newFilesystemProbe() *filesystemProbe {
	return &filesystemProbe{
		active: make(map[probeKey]*probeCall), changed: make(chan struct{}), inspect: inspectDirectory,
	}
}

func inspectDirectory(path string, canonicalize bool) (string, error) {
	if canonicalize {
		var err error
		path, err = filepath.EvalSymlinks(path)
		if err != nil {
			return "", err
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("root is not a directory")
	}
	return path, nil
}

func (p *filesystemProbe) check(ctx context.Context, path string, canonicalize bool) (string, error) {
	key := probeKey{path: path, canonicalize: canonicalize}
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		p.mu.Lock()
		call := p.active[key]
		if call == nil && len(p.active) < maxFilesystemProbes {
			call = &probeCall{done: make(chan struct{})}
			p.active[key] = call
			go func() {
				call.path, call.err = p.inspect(path, canonicalize)
				p.mu.Lock()
				delete(p.active, key)
				close(call.done)
				close(p.changed)
				p.changed = make(chan struct{})
				p.mu.Unlock()
			}()
		}
		changed := p.changed
		p.mu.Unlock()
		if call != nil {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-call.done:
				if err := ctx.Err(); err != nil {
					return "", err
				}
				return call.path, call.err
			}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-changed:
		}
	}
}
