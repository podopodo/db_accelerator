package lifecycle

import (
	"testing"
	"time"
)

func TestLifecycleHappyPath(t *testing.T) {
	now := time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC)
	manager := New(now)
	if manager.IsReady() || !manager.IsLive() {
		t.Fatal("starting readiness is wrong")
	}
	if err := manager.Transition(Ready, "ready", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if !manager.IsReady() {
		t.Fatal("ready state not reported")
	}
	if err := manager.Transition(Draining, "stop", now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := manager.Transition(Stopped, "done", now.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	if manager.IsLive() || manager.IsReady() {
		t.Fatal("stopped state reports healthy")
	}
}

func TestLifecycleRejectsRestartFromStopped(t *testing.T) {
	manager := New(time.Now())
	if err := manager.Transition(Stopped, "done", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := manager.Transition(Ready, "illegal", time.Now()); err == nil {
		t.Fatal("illegal transition accepted")
	}
}
