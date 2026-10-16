//go:build linux

package display

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLinuxCreateAppliesModeAndDestroys(t *testing.T) {
	var calls []string
	failDoctor := 2
	v := &vdisplay{sleep: func(time.Duration) {}, run: func(name string, args ...string) (string, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		if name == "kscreen-doctor" && failDoctor > 0 {
			failDoctor--
			return "no such output", errors.New("exit 1")
		}
		if len(args) > 2 && args[2] == "create" {
			return "noise\nVirtual-1", nil
		}
		return "", nil
	}}
	if err := v.create(Mode{1920, 1080, 60}); err != nil {
		t.Fatal(err)
	}
	if v.output != "Virtual-1" || len(calls) != 4 {
		t.Fatalf("%q %v", v.output, calls)
	}
	if want := "kscreen-doctor output.Virtual-1.enable output.Virtual-1.mode.1920x1080@60 output.Virtual-1.scale.1"; calls[3] != want {
		t.Fatalf("got %q", calls[3])
	}
	if err := v.destroy(); err != nil || v.output != "" {
		t.Fatal(err)
	}
}

func TestLinuxCreateFailureCleansUp(t *testing.T) {
	var destroyed bool
	v := &vdisplay{sleep: func(time.Duration) {}, run: func(name string, args ...string) (string, error) {
		if len(args) > 2 && args[2] == "destroy" {
			destroyed = true
			return "", nil
		}
		if name == "kscreen-doctor" {
			return "", errors.New("exit 1")
		}
		return "Virtual-1", nil
	}}
	if err := v.create(Mode{1920, 1080, 60}); err == nil || !destroyed {
		t.Fatalf("err=%v destroyed=%v", err, destroyed)
	}
}

func TestLinuxHelperMissing(t *testing.T) {
	v := &vdisplay{run: func(string, ...string) (string, error) { return "sudo: a password is required", errors.New("exit 1") }}
	if err := v.create(Mode{1920, 1080, 60}); err == nil || !strings.Contains(err.Error(), "installer") {
		t.Fatal(err)
	}
}

func TestSessionEnvFillsMissingWayland(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "wayland-0"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "wayland-0.lock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(sessionEnv([]string{"XDG_RUNTIME_DIR=" + dir}), "\n")
	for _, want := range []string{"WAYLAND_DISPLAY=wayland-0\n", "QT_QPA_PLATFORM=wayland"} {
		if !strings.Contains(got+"\n", want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	kept := strings.Join(sessionEnv([]string{"XDG_RUNTIME_DIR=" + dir, "WAYLAND_DISPLAY=wayland-9", "QT_QPA_PLATFORM=xcb"}), "\n")
	if strings.Contains(kept, "wayland-0") || strings.Count(kept, "QT_QPA_PLATFORM") != 1 {
		t.Errorf("existing values must be kept: %q", kept)
	}
}
