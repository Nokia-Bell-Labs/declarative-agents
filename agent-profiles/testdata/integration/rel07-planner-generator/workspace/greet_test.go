package greet

import "testing"

func TestHello(t *testing.T) {
	if got, want := Hello("Planner"), "Hello, Planner!"; got != want {
		t.Fatalf("Hello() = %q, want %q", got, want)
	}
}
