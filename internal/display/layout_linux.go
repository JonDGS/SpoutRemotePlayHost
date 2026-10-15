//go:build linux

package display

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// kscreenOutput is the part of `kscreen-doctor -j` the host needs.
type kscreenOutput struct {
	Name          string `json:"name"`
	Enabled       bool   `json:"enabled"`
	Connected     bool   `json:"connected"`
	Priority      int    `json:"priority"`
	CurrentModeID string `json:"currentModeId"`
	Pos           struct {
		X int `json:"x"`
		Y int `json:"y"`
	} `json:"pos"`
	Size struct {
		Width  int `json:"width"`
		Height int `json:"height"`
	} `json:"size"`
}

func isVirtual(name string) bool { return strings.HasPrefix(name, "Virtual-") }

func readLayout(run runner) ([]kscreenOutput, error) {
	out, err := run("kscreen-doctor", "-j")
	if err != nil {
		return nil, fmt.Errorf("kscreen-doctor -j: %w: %s", err, out)
	}
	// kscreen-doctor may print warnings before the JSON.
	if i := strings.IndexByte(out, '{'); i > 0 {
		out = out[i:]
	}
	var cfg struct {
		Outputs []kscreenOutput `json:"outputs"`
	}
	if err := json.Unmarshal([]byte(out), &cfg); err != nil {
		return nil, fmt.Errorf("parse kscreen-doctor -j: %w", err)
	}
	return cfg.Outputs, nil
}

// physical returns the connected, enabled outputs that are not virtual monitors.
func physical(outs []kscreenOutput) []kscreenOutput {
	var list []kscreenOutput
	for _, o := range outs {
		if o.Connected && o.Enabled && !isVirtual(o.Name) {
			list = append(list, o)
		}
	}
	return list
}

// Outputs lists the physical displays. On Linux the ID is the KWin output name (e.g. DP-1).
func Outputs() ([]Output, error) {
	outs, err := readLayout(execRunner)
	if err != nil {
		return nil, err
	}
	var list []Output
	for _, o := range outs {
		if !o.Connected || isVirtual(o.Name) {
			continue
		}
		list = append(list, Output{ID: o.Name, Name: o.Name, Device: o.Name,
			Width: o.Size.Width, Height: o.Size.Height, Primary: o.Priority == 1})
	}
	return list, nil
}

// layoutFile keeps the layout from before a session so it can be restored after a crash.
func layoutFile() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "SpoutRemotePlayHost", "linux-layout.json")
}

func saveLayout(path string, outs []kscreenOutput) error {
	data, err := json.Marshal(outs)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func loadLayout(path string) ([]kscreenOutput, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var outs []kscreenOutput
	return outs, json.Unmarshal(data, &outs)
}

// isolateArgs makes the virtual monitor the main display and turns off every physical
// display that is not in keep.
func isolateArgs(virt string, saved []kscreenOutput, keep []string) []string {
	kept := map[string]bool{}
	for _, id := range keep {
		kept[id] = true
	}
	args := []string{fmt.Sprintf("output.%s.priority.1", virt)}
	for _, o := range saved {
		if !kept[o.Name] {
			args = append(args, fmt.Sprintf("output.%s.disable", o.Name))
		}
	}
	return args
}

// restoreArgs turns the saved displays back on with their mode, position and priority.
func restoreArgs(saved []kscreenOutput) []string {
	outs := append([]kscreenOutput(nil), saved...)
	sort.SliceStable(outs, func(i, j int) bool { return outs[i].Priority < outs[j].Priority })
	var args []string
	for _, o := range outs {
		args = append(args, fmt.Sprintf("output.%s.enable", o.Name))
		if o.CurrentModeID != "" {
			args = append(args, fmt.Sprintf("output.%s.mode.%s", o.Name, o.CurrentModeID))
		}
		args = append(args, fmt.Sprintf("output.%s.position.%d,%d", o.Name, o.Pos.X, o.Pos.Y))
		if o.Priority > 0 {
			args = append(args, fmt.Sprintf("output.%s.priority.%d", o.Name, o.Priority))
		}
	}
	return args
}

// applyRestore turns the saved displays back on and checks that KWin did it:
// kscreen-doctor exits 0 even when it could not apply a change.
func applyRestore(run runner, saved []kscreenOutput) error {
	if out, err := run("kscreen-doctor", restoreArgs(saved)...); err != nil {
		return fmt.Errorf("%w: %s", err, out)
	}
	now, err := readLayout(run)
	if err != nil {
		return err
	}
	on := map[string]bool{}
	for _, o := range now {
		on[o.Name] = o.Enabled
	}
	var off []string
	for _, o := range saved {
		if !on[o.Name] {
			off = append(off, o.Name)
		}
	}
	if len(off) > 0 {
		return fmt.Errorf("still off after restore: %s", strings.Join(off, ", "))
	}
	return nil
}

// recoverLayout restores a layout left behind by a host that stopped mid-session. The
// service can start before the desktop session, so it retries for a while.
func (v *vdisplay) recoverLayout(attempts int, wait time.Duration) {
	saved, err := loadLayout(v.layoutPath)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil || len(saved) == 0 {
		log.Println("ignoring unreadable saved display layout:", err)
		_ = os.Remove(v.layoutPath)
		return
	}
	for i := 0; i < attempts; i++ {
		err := applyRestore(v.run, saved)
		if err == nil {
			log.Println("restored the display layout from an interrupted session")
			_ = os.Remove(v.layoutPath)
			return
		}
		if i == attempts-1 {
			log.Println("could not restore the display layout from an interrupted session:", err)
			return
		}
		v.sleep(wait)
	}
}
