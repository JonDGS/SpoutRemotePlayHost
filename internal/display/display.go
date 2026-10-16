package display

import (
	"errors"
	"sync"
)

var ErrNotImplemented = errors.New("virtual display driver not implemented on this platform yet")

// Mode describes the virtual monitor requested for a streaming session.
type Mode struct {
	Width, Height, RefreshHz int
}

// Output is a physical display of this PC.
type Output struct {
	ID      string `json:"id"`     // stable across sessions and reboots
	Name    string `json:"name"`   // monitor model, e.g. "LG ULTRAWIDE"
	Device  string `json:"device"` // OS name, e.g. \\.\DISPLAY2
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	Primary bool   `json:"primary"`
}

// KeepOutputs, when set, returns the IDs of displays left on during a session; every other
// physical display is turned off while the virtual monitor exists (where supported).
var KeepOutputs func() []string

func keepOutputs() []string {
	if KeepOutputs == nil {
		return nil
	}
	return KeepOutputs()
}

// Persistent, when set before New, keeps one virtual monitor connected for the whole time
// the host runs (Linux only; ignored elsewhere), instead of creating one per session.
//
// Steam on Wayland captures through the xdg-desktop-portal. It opens that capture when the
// stream starts, before the host sees the session, and the portal grant is tied to one
// screen. A monitor that only appears once the session starts is never captured, and a
// "Full Workspace" grant breaks as soon as the other displays turn off. With a persistent
// monitor the user grants Steam that screen once; sessions then only change its mode,
// make it primary and turn the other displays off.
var Persistent *Mode

// Manager creates and removes the virtual monitor used for a session.
type Manager interface {
	Create(Mode) error
	Destroy() error
	Active() bool
}

// backend is the OS-specific driver layer.
type backend interface {
	create(Mode) error
	destroy() error
}

type manager struct {
	mu     sync.Mutex
	b      backend
	active bool
}

func (m *manager) Create(mode Mode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active {
		if err := m.b.destroy(); err != nil {
			return err
		}
		m.active = false
	}
	if err := m.b.create(mode); err != nil {
		return err
	}
	m.active = true
	return nil
}

func (m *manager) Destroy() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.active {
		return nil
	}
	if err := m.b.destroy(); err != nil {
		return err
	}
	m.active = false
	return nil
}

func (m *manager) Active() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active
}

type stub struct{}

func (stub) create(Mode) error { return ErrNotImplemented }
func (stub) destroy() error    { return nil }

// New returns the Manager for the current OS.
func New() Manager { return &manager{b: newBackend()} }

func newWithBackend(b backend) Manager { return &manager{b: b} }
