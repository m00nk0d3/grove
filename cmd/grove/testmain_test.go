package main

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "grove-tests-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "create test home:", err)
		os.Exit(1)
	}
	if err := os.Setenv("HOME", home); err != nil {
		fmt.Fprintln(os.Stderr, "set test HOME:", err)
		os.Exit(1)
	}
	if err := os.Setenv("USERPROFILE", home); err != nil {
		fmt.Fprintln(os.Stderr, "set test USERPROFILE:", err)
		os.Exit(1)
	}

	code := m.Run()
	if err := os.RemoveAll(home); err != nil && code == 0 {
		fmt.Fprintln(os.Stderr, "remove test home:", err)
		code = 1
	}
	os.Exit(code)
}
