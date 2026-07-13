package cli

import "testing"

func TestTerminalJob(t *testing.T) {
	cases := []struct {
		status   string
		done, ok bool
	}{
		{"queued", false, false},
		{"leased", false, false},
		{"running", false, false},
		{"succeeded", true, true},
		{"failed", true, false},
		{"dead", true, false},
		{"", false, false},
	}
	for _, tc := range cases {
		done, ok := terminalJob(tc.status)
		if done != tc.done || ok != tc.ok {
			t.Fatalf("%q: got done=%v ok=%v want done=%v ok=%v", tc.status, done, ok, tc.done, tc.ok)
		}
	}
}
