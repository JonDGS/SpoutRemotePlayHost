//go:build linux

package display

import (
	"strings"
	"testing"
	"time"
)

func countPrefix(calls []string, prefix string) int {
	n := 0
	for _, c := range calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func TestLinuxPersistentMonitorSurvivesSessions(t *testing.T) {
	f := &fakeKWin{layout: twoMonitors}
	v := newTestDisplay(t, f)
	v.persistent = &Mode{2560, 1600, 144}

	v.startPersistent(*v.persistent, 3, time.Second)
	if want := "kscreen-doctor output.Virtual-5.enable output.Virtual-5.mode.2560x1600@144 output.Virtual-5.scale.1 output.Virtual-5.position.5120,0"; f.last("kscreen-doctor output.Virtual-5.enable") != want {
		t.Fatalf("must sit right of the other displays, not primary: %q", f.calls)
	}
	if countPrefix(f.calls, "kscreen-doctor output.Virtual-5.priority") != 0 {
		t.Fatal("must not become primary outside a session")
	}

	for i := 0; i < 2; i++ {
		if err := v.create(Mode{1280, 800, 60}); err != nil {
			t.Fatal(err)
		}
		if f.last("kscreen-doctor output.Virtual-5.mode") != "kscreen-doctor output.Virtual-5.mode.1280x800@60" {
			t.Fatalf("session %d must set the mode through KWin: %q", i, f.calls)
		}
		if f.last("kscreen-doctor output.Virtual-5.priority") != "kscreen-doctor output.Virtual-5.priority.1 output.DP-3.disable output.DP-1.disable" {
			t.Fatalf("session %d must isolate: %q", i, f.calls)
		}
		if err := v.destroy(); err != nil {
			t.Fatal(err)
		}
		if len(f.off) != 0 {
			t.Fatalf("session %d must restore the displays: off=%v", i, f.off)
		}
	}
	if n := countPrefix(f.calls, "sudo -n "+helperPath+" create"); n != 1 {
		t.Fatalf("the monitor must be created once, got %d", n)
	}
	if countPrefix(f.calls, "sudo -n "+helperPath+" destroy") != 0 {
		t.Fatal("ending a session must not remove the persistent monitor")
	}
}

func TestLinuxPersistentMonitorRetriesUntilSessionIsUp(t *testing.T) {
	f := &fakeKWin{layout: "no session yet"}
	v := newTestDisplay(t, f)
	v.persistent = &Mode{1920, 1080, 60}
	tries := 0
	v.sleep = func(time.Duration) {
		if tries++; tries == 2 {
			f.layout = twoMonitors
		}
	}
	v.startPersistent(*v.persistent, 5, time.Second)
	if v.output != "Virtual-5" {
		t.Fatalf("output=%q calls=%q", v.output, f.calls)
	}
}
