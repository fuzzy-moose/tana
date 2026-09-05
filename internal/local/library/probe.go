package library

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"sync"
)

const maxFilesystemProbes = 4

type probeCall struct {
	done chan struct{}
	err  error
}

// Filesystem calls cannot reliably be canceled on a stalled NAS. Retain their
// slots until they actually return, and share in-flight work for the same root.
// Callers can stop waiting without spawning more work beyond this fixed bound.
type filesystemProbe struct {
	mu      sync.Mutex
	active  map[string]*probeCall
	changed chan struct{}
	inspect func(string) error
}

func newFilesystemProbe(dirFS func(string) fs.FS) *filesystemProbe {
	return &filesystemProbe{
		active: make(map[string]*probeCall), changed: make(chan struct{}),
		inspect: func(path string) error {
			return inspectDirectory(dirFS, path)
		},
	}
}

func inspectDirectory(dirFS func(string) fs.FS, path string) error {
	// Stat from the parent so inspecting the root does not require search
	// permission inside it.
	clean := filepath.Clean(path)
	parent, name := filepath.Dir(clean), filepath.Base(clean)
	if parent == clean {
		name = "."
	}
	info, err := fs.Stat(dirFS(parent), name)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("root is not a directory")
	}
	return nil
}

func (p *filesystemProbe) check(ctx context.Context, path string) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		p.mu.Lock()
		call := p.active[path]
		if call == nil && len(p.active) < maxFilesystemProbes {
			call = &probeCall{done: make(chan struct{})}
			p.active[path] = call
			go func() {
				call.err = p.inspect(path)
				p.mu.Lock()
				delete(p.active, path)
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
				return ctx.Err()
			case <-call.done:
				if err := ctx.Err(); err != nil {
					return err
				}
				return call.err
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}
