package timeout_test

import (
	"testing"

	"goblin/pkg/applets/timeout"
	"goblin/pkg/core"
	"goblin/pkg/testutil"
)

func TestTimeout(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "missing",
			Args:     []string{},
			WantCode: core.ExitUsage,
		},
		{
			Name:     "invalid_duration",
			Args:     []string{"1x", "echo"},
			WantCode: core.ExitUsage,
		},
		{
			Name:     "basic",
			Args:     []string{"1", "echo", "ok"},
			WantCode: core.ExitSuccess,
		},
	}
	testutil.RunAppletTests(t, timeout.Run, tests)
}
