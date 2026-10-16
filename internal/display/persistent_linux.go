//go:build linux

package display

import (
	"fmt"
	"log"
	"time"
)

// ensurePersistent creates the persistent monitor if it does not exist yet, to the right
// of the other displays and not primary, so the desktop is unchanged between sessions.
func (v *vdisplay) ensurePersistent(m Mode) error {
	if v.output != "" {
		return nil
	}
	outs, err := readLayout(v.run)
	if err != nil {
		return err
	}
	right := 0
	for _, o := range physical(outs) {
		if x := o.Pos.X + o.Size.Width; x > right {
			right = x
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
	args := []string{
		fmt.Sprintf("output.%s.enable", name),
		fmt.Sprintf("output.%s.mode.%dx%d@%d", name, m.Width, m.Height, m.RefreshHz),
		fmt.Sprintf("output.%s.scale.1", name),
		fmt.Sprintf("output.%s.position.%d,0", name, right),
	}
	var last string
	for i := 0; i < 20; i++ {
		if last, err = v.run("kscreen-doctor", args...); err == nil {
			v.output = name
			log.Printf("persistent virtual monitor %s at %d,0 (%dx%d@%d)", name, right, m.Width, m.Height, m.RefreshHz)
			return nil
		}
		v.sleep(250 * time.Millisecond)
	}
	_, _ = v.run("sudo", "-n", helperPath, "destroy")
	return fmt.Errorf("could not set up the persistent monitor %s: %s", name, last)
}

// startPersistent sets the monitor up once the desktop session is reachable.
func (v *vdisplay) startPersistent(m Mode, attempts int, wait time.Duration) {
	for i := 0; i < attempts; i++ {
		v.mu.Lock()
		err := v.ensurePersistent(m)
		v.mu.Unlock()
		if err == nil {
			return
		}
		if i == attempts-1 {
			log.Println("persistent virtual monitor:", err)
			return
		}
		v.sleep(wait)
	}
}

// createPersistent starts a session on the persistent monitor: same output, new mode.
func (v *vdisplay) createPersistent(m Mode) error {
	outs, err := readLayout(v.run)
	if err != nil {
		return err
	}
	if err := v.ensurePersistent(*v.persistent); err != nil {
		return err
	}
	name := v.output
	// Changing the mode through KWin keeps the output (and Steam's capture of it) alive;
	// replugging the connector would end the capture.
	if out, err := v.run("kscreen-doctor", fmt.Sprintf("output.%s.mode.%dx%d@%d", name, m.Width, m.Height, m.RefreshHz)); err != nil {
		log.Printf("could not set %dx%d@%d on %s, keeping its current mode: %v: %s", m.Width, m.Height, m.RefreshHz, name, err, out)
	}
	v.isolate(name, physical(outs))
	return nil
}

// destroyPersistent ends a session: the other displays come back and become primary
// again, and the virtual monitor stays connected for the next session.
func (v *vdisplay) destroyPersistent() error {
	if err := v.restore(); err != nil {
		log.Println(err)
		return err
	}
	return nil
}
