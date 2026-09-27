package app

import (
	"context"
	"os"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type wailsEmitter struct {
	mu  sync.RWMutex
	ctx context.Context
}

func (e *wailsEmitter) setContext(ctx context.Context) {
	e.mu.Lock()
	e.ctx = ctx
	e.mu.Unlock()
}

func (e *wailsEmitter) Emit(name string, payload any) {
	e.mu.RLock()
	ctx := e.ctx
	e.mu.RUnlock()
	if ctx != nil {
		runtime.EventsEmit(ctx, name, payload)
	}
}

// Application owns runtime lifecycle concerns but is not itself exposed to Wails.
type Application struct {
	emitter  *wailsEmitter
	service  *Service
	bindings *Bindings
}

// NewApplication wires the NFC application service to the desktop runtime.
func NewApplication() *Application {
	configureLibNFCDiscovery()
	emitter := &wailsEmitter{}
	service := NewService(emitter)
	service.setDialogs(wailsFileDialogs{emitter: emitter})
	return &Application{
		emitter:  emitter,
		service:  service,
		bindings: &Bindings{service: service},
	}
}

// configureLibNFCDiscovery supplies application-local defaults that would
// otherwise come from a system libnfc.conf. NFCX deliberately does not read or
// modify that global file. Explicit user environment values always win.
func configureLibNFCDiscovery() {
	for name, value := range map[string]string{
		"LIBNFC_AUTO_SCAN":      "true",
		"LIBNFC_INTRUSIVE_SCAN": "true",
	} {
		if _, exists := os.LookupEnv(name); !exists {
			_ = os.Setenv(name, value)
		}
	}
}

// Startup attaches the Wails context used only for emitting GUI events.
func (a *Application) Startup(ctx context.Context) {
	a.emitter.setContext(ctx)
	a.bindings.quit = func() { runtime.Quit(ctx) }
	a.service.CheckUpdatesInBackground()
}

// Shutdown stops background work during application teardown.
func (a *Application) Shutdown(context.Context) {
	a.service.Shutdown()
}

// Bindings returns the narrow application API exposed to the GUI.
func (a *Application) Bindings() *Bindings {
	return a.bindings
}
