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

const twoMonitors = `kscreen warning
{"outputs":[
 {"name":"DP-3","enabled":true,"connected":true,"priority":2,"currentModeId":"2","pos":{"x":2560,"y":0},"size":{"width":2560,"height":1440},
  "modes":[{"id":"1","refreshRate":59.95,"size":{"width":2560,"height":1440}},{"id":"2","refreshRate":143.97,"size":{"width":2560,"height":1440}}]},
 {"name":"DP-1","enabled":true,"connected":true,"priority":1,"currentModeId":"34","pos":{"x":0,"y":0},"size":{"width":2560,"height":1440},
  "modes":[{"id":"33","refreshRate":59.95,"size":{"width":2560,"height":1440}},{"id":"34","refreshRate":143.97,"size":{"width":2560,"height":1440}}]},
 {"name":"HDMI-A-1","enabled":false,"connected":true,"priority":0},
 {"name":"Virtual-1","enabled":false,"connected":false,"priority":0}]}`

// KWin renumbers modes once the virtual monitor exists: DP-1's 144 Hz mode is 83 now and
// 34 belongs to the virtual output. Seen on a real Plasma 6.7 session.
var renumbered = strings.NewReplacer(`"id":"33"`, `"id":"82"`, `"id":"34"`, `"id":"83"`)

// fakeKWin records kscreen-doctor calls, tracks which outputs are on and answers -j.
// Like the real tool, a call it cannot apply still exits 0 unless exitErr is set.
type fakeKWin struct {
	calls   []string
	layout  string
	failArg string // ignore any kscreen-doctor call containing this argument
	exitErr bool   // and also return an error for it
	off     map[string]bool
	created bool // the virtual monitor exists, so mode IDs are renumbered
}

func (f *fakeKWin) run(name string, args ...string) (string, error) {
	line := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, line)
	switch {
	case name == "kscreen-doctor" && len(args) == 1 && args[0] == "-j":
		out := f.layout
		if f.created {
			out = renumbered.Replace(out)
		}
		for n := range f.off {
			out = strings.Replace(out, `"name":"`+n+`","enabled":true`, `"name":"`+n+`","enabled":false`, 1)
		}
		return out, nil
	case name == "kscreen-doctor" && f.failArg != "" && strings.Contains(line, f.failArg):
		if f.exitErr {
			return "failed", errors.New("exit 1")
		}
		return "Output not found", nil
	case name == "kscreen-doctor":
		if f.off == nil {
			f.off = map[string]bool{}
		}
		for _, a := range args {
			p := strings.Split(a, ".")
			if len(p) == 3 && p[2] == "disable" {
				f.off[p[1]] = true
			} else if len(p) == 3 && p[2] == "enable" {
				delete(f.off, p[1])
			}
		}
	case len(args) > 2 && args[2] == "create":
		f.created = true
		return "Virtual-5", nil
	}
	return "", nil
}

func (f *fakeKWin) last(prefix string) string {
	for i := len(f.calls) - 1; i >= 0; i-- {
		if strings.HasPrefix(f.calls[i], prefix) {
			return f.calls[i]
		}
	}
	return ""
}

func newTestDisplay(t *testing.T, f *fakeKWin, keep ...string) *vdisplay {
	return &vdisplay{run: f.run, sleep: func(time.Duration) {}, keep: func() []string { return keep },
		layoutPath: filepath.Join(t.TempDir(), "layout.json")}
}

func TestLinuxSessionTurnsOtherDisplaysOffAndRestores(t *testing.T) {
	f := &fakeKWin{layout: twoMonitors}
	v := newTestDisplay(t, f)
	if err := v.create(Mode{2560, 1600, 144}); err != nil {
		t.Fatal(err)
	}
	if want := "kscreen-doctor output.Virtual-5.priority.1 output.DP-3.disable output.DP-1.disable"; f.calls[len(f.calls)-1] != want {
		t.Fatalf("isolate: got %q", f.calls[len(f.calls)-1])
	}
	if _, err := os.Stat(v.layoutPath); err != nil {
		t.Fatal("layout must be saved during a session:", err)
	}
	if err := v.destroy(); err != nil {
		t.Fatal(err)
	}
	n := len(f.calls)
	want := "kscreen-doctor output.DP-1.enable output.DP-1.mode.83 output.DP-1.position.0,0 output.DP-1.priority.1 " +
		"output.DP-3.enable output.DP-3.mode.2 output.DP-3.position.2560,0 output.DP-3.priority.2"
	if f.calls[n-3] != want || f.calls[n-2] != "kscreen-doctor -j" || !strings.HasSuffix(f.calls[n-1], "destroy") {
		t.Fatalf("restore, check, then destroy: %q", f.calls[n-3:])
	}
	if _, err := os.Stat(v.layoutPath); !os.IsNotExist(err) {
		t.Fatal("layout file must be removed after a restore")
	}
}

func TestLinuxKeepsChosenDisplaysOn(t *testing.T) {
	f := &fakeKWin{layout: twoMonitors}
	v := newTestDisplay(t, f, "DP-3")
	if err := v.create(Mode{1280, 800, 60}); err != nil {
		t.Fatal(err)
	}
	if got := f.last("kscreen-doctor output.Virtual-5.priority"); got != "kscreen-doctor output.Virtual-5.priority.1 output.DP-1.disable" {
		t.Fatalf("got %q", got)
	}
}

func TestLinuxIsolateFailureLeavesDisplaysOn(t *testing.T) {
	f := &fakeKWin{layout: twoMonitors, failArg: "disable", exitErr: true}
	v := newTestDisplay(t, f)
	if err := v.create(Mode{1280, 800, 60}); err != nil {
		t.Fatal("the stream must still start:", err)
	}
	if !strings.Contains(f.last("kscreen-doctor output.DP-1.enable"), "output.DP-1.enable") || v.saved != nil {
		t.Fatalf("expected a restore after the failed isolate: %q", f.calls)
	}
}

func TestLinuxUnreadableLayoutSkipsIsolation(t *testing.T) {
	f := &fakeKWin{layout: "not json"}
	v := newTestDisplay(t, f)
	if err := v.create(Mode{1280, 800, 60}); err != nil {
		t.Fatal(err)
	}
	if f.last("kscreen-doctor output.Virtual-5.priority") != "" {
		t.Fatal("must not turn displays off without a layout to restore")
	}
}

func TestLinuxRestoreFailureKeepsLayoutForRecovery(t *testing.T) {
	f := &fakeKWin{layout: twoMonitors}
	v := newTestDisplay(t, f)
	if err := v.create(Mode{1280, 800, 60}); err != nil {
		t.Fatal(err)
	}
	// kscreen-doctor exits 0 but the displays stay off: the read-back must notice.
	f.failArg = "enable"
	if err := v.destroy(); err == nil {
		t.Fatal("destroy must report the failed restore")
	}
	if f.last("sudo") != "sudo -n "+helperPath+" create 1280 800 60" {
		t.Fatalf("the virtual monitor must stay when the displays did not come back: %q", f.calls)
	}
	if _, err := os.Stat(v.layoutPath); err != nil {
		t.Fatal("a failed restore must keep the layout for the next start:", err)
	}

	// The next host start restores it once KWin answers.
	f2 := &fakeKWin{layout: twoMonitors, failArg: "enable", off: map[string]bool{"DP-1": true, "DP-3": true}, created: true}
	v2 := &vdisplay{run: f2.run, sleep: func(time.Duration) {}, layoutPath: v.layoutPath}
	tries := 0
	v2.run = func(name string, args ...string) (string, error) {
		if name == "kscreen-doctor" && args[0] != "-j" {
			if tries++; tries == 3 {
				f2.failArg = ""
			}
		}
		return f2.run(name, args...)
	}
	v2.recoverLayout(5, time.Second)
	if tries != 3 || len(f2.off) != 0 {
		t.Fatalf("tries=%d off=%v calls=%q", tries, f2.off, f2.calls)
	}
	if _, err := os.Stat(v.layoutPath); !os.IsNotExist(err) {
		t.Fatal("layout file must be removed after recovery")
	}
}

func TestLinuxOutputsSkipVirtualAndDisconnected(t *testing.T) {
	outs, err := readLayout((&fakeKWin{layout: twoMonitors}).run)
	if err != nil {
		t.Fatal(err)
	}
	if got := physical(outs); len(got) != 2 || got[0].Name != "DP-3" || got[1].Name != "DP-1" {
		t.Fatalf("%+v", got)
	}
}
