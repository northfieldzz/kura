package config

import (
	"os"
	"strings"
	"testing"
)

func clearEnv(t *testing.T) {
	t.Helper()
	env := os.Environ()
	t.Cleanup(func() {
		os.Clearenv()
		for _, e := range env {
			if i := strings.Index(e, "="); i != -1 {
				os.Setenv(e[:i], e[i+1:])
			}
		}
	})
	os.Clearenv()
}
