//go:build !windows && !linux

package display

// Outputs lists the physical displays. Turning displays off during a session is Windows-only for now.
func Outputs() ([]Output, error) { return nil, ErrNotImplemented }
