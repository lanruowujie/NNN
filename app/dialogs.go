package app

import (
	"context"
	"errors"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

var ErrDialogUnavailable = errors.New("file dialog is unavailable")

type fileDialogs interface {
	OpenFile(title, pattern string) (string, error)
	SaveFile(title, defaultName, pattern string) (string, error)
}

type noFileDialogs struct{}

func (noFileDialogs) OpenFile(string, string) (string, error) { return "", ErrDialogUnavailable }
func (noFileDialogs) SaveFile(string, string, string) (string, error) {
	return "", ErrDialogUnavailable
}

type wailsFileDialogs struct{ emitter *wailsEmitter }

func (d wailsFileDialogs) OpenFile(title, pattern string) (string, error) {
	ctx := d.context()
	if ctx == nil {
		return "", ErrDialogUnavailable
	}
	return runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{
		Title: title, Filters: []runtime.FileFilter{{DisplayName: title, Pattern: pattern}},
	})
}

func (d wailsFileDialogs) SaveFile(title, defaultName, pattern string) (string, error) {
	ctx := d.context()
	if ctx == nil {
		return "", ErrDialogUnavailable
	}
	return runtime.SaveFileDialog(ctx, runtime.SaveDialogOptions{
		Title: title, DefaultFilename: defaultName,
		Filters: []runtime.FileFilter{{DisplayName: title, Pattern: pattern}},
	})
}

func (d wailsFileDialogs) context() context.Context {
	if d.emitter == nil {
		return nil
	}
	d.emitter.mu.RLock()
	defer d.emitter.mu.RUnlock()
	return d.emitter.ctx
}
