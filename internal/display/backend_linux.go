//go:build linux

package display

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const helperPath = "/usr/local/libexec/spout-vdisplay"

// runner executes a command and returns its combined output.
type runner func(name string, args ...string) (string, error)

func execRunner(name string, args ...string) (string, error) {
	var out bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Env = sessionEnv(os.Environ())
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}

// sessionEnv makes sure Wayland tools can reach the compositor. The service may start before
// the desktop session exports its environment, and Qt aborts without a Wayland display.
func sessionEnv(env []string) []string {
	has := func(k string) bool {
		for _, e := range env {
			if strings.HasPrefix(e, k+"=") {
				return true
			}
		}
		return false
	}
	if !has("XDG_RUNTIME_DIR") {
		env = append(env, fmt.Sprintf("XDG_RUNTIME_DIR=/run/user/%d", os.Getuid()))
	}
	if !has("WAYLAND_DISPLAY") {
		dir := ""
		for _, e := range env {
			if strings.HasPrefix(e, "XDG_RUNTIME_DIR=") {
				dir = strings.TrimPrefix(e, "XDG_RUNTIME_DIR=")
			}
		}
		socks, _ := filepath.Glob(filepath.Join(dir, "wayland-[0-9]*"))
		for _, sock := range socks {
			if !strings.HasSuffix(sock, ".lock") {
				env = append(env, "WAYLAND_DISPLAY="+filepath.Base(sock))
				break
			}
		}
	}
	if !has("QT_QPA_PLATFORM") {
		env = append(env, "QT_QPA_PLATFORM=wayland")
	}
	return env
}

// vdisplay drives the vibeshine_drm kernel module through a root helper (sudoers
// allows only that helper) and then applies the mode to the KWin output with kscreen-doctor.
// While it exists, the virtual monitor is the main display and the other displays are off.
type vdisplay struct {
	run    runner
	sleep  func(time.Duration)
	keep   func() []string
	output string

	layoutPath string          // where the pre-session layout is kept; empty disables isolation
	saved      []kscreenOutput // displays turned off for this session
}

func newBackend() backend {
	v := &vdisplay{run: execRunner, sleep: time.Sleep, keep: keepOutputs, layoutPath: layoutFile()}
	go v.recoverLayout(24, 5*time.Second)
	return v
}

func (v *vdisplay) create(m Mode) error {
	// Read the layout before the virtual monitor appears, so it can be put back afterwards.
	var before []kscreenOutput
	if v.layoutPath != "" {
		outs, err := readLayout(v.run)
		if err != nil {
			log.Println("not turning other displays off:", err)
		} else {
			before = physical(outs)
		}
	}
	out, err := v.run("sudo", "-n", helperPath, "create",
		fmt.Sprint(m.Width), fmt.Sprint(m.Height), fmt.Sprint(m.RefreshHz))
	if err != nil {
		return fmt.Errorf("virtual display driver: %w: %s (run the Linux installer to set up the driver)", err, out)
	}
	name := lastLine(out)
	if name == "" {
		return fmt.Errorf("virtual display helper returned no connector name")
	}
	v.output = name

	// KWin picks up the hotplug asynchronously; the output may not be listed for a moment.
	arg := fmt.Sprintf("output.%s.mode.%dx%d@%d", name, m.Width, m.Height, m.RefreshHz)
	var last string
	for i := 0; i < 20; i++ {
		last, err = v.run("kscreen-doctor", fmt.Sprintf("output.%s.enable", name), arg)
		if err == nil {
			v.isolate(name, before)
			return nil
		}
		v.sleep(250 * time.Millisecond)
	}
	_, _ = v.run("sudo", "-n", helperPath, "destroy")
	v.output = ""
	return fmt.Errorf("could not apply %dx%d@%d to %s: %s", m.Width, m.Height, m.RefreshHz, name, last)
}

// isolate makes the virtual monitor the main display and turns the other displays off.
// A failure leaves the displays on: the stream still works, just not on its own.
func (v *vdisplay) isolate(virt string, before []kscreenOutput) {
	if len(before) == 0 {
		return
	}
	var keep []string
	if v.keep != nil {
		keep = v.keep()
	}
	if err := saveLayout(v.layoutPath, before); err != nil {
		log.Println("not turning other displays off: could not save the layout:", err)
		return
	}
	v.saved = before
	if out, err := v.run("kscreen-doctor", isolateArgs(virt, before, keep)...); err != nil {
		log.Printf("could not turn other displays off: %v: %s", err, out)
		v.restore()
	}
}

// restore puts back the layout saved by isolate. On failure the saved file stays, so the
// next start of the host tries again.
func (v *vdisplay) restore() {
	if len(v.saved) == 0 {
		return
	}
	if err := applyRestore(v.run, v.saved); err != nil {
		log.Println("could not restore the display layout:", err)
		return
	}
	v.saved = nil
	_ = os.Remove(v.layoutPath)
}

func (v *vdisplay) destroy() error {
	// Turn the real displays back on before the virtual one goes, so KWin always has one.
	v.restore()
	out, err := v.run("sudo", "-n", helperPath, "destroy")
	if err != nil {
		return fmt.Errorf("destroy virtual display: %w: %s", err, out)
	}
	v.output = ""
	return nil
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[i+1:])
	}
	return s
}
