package killall_test

import (
	"testing"

	"goblin/pkg/applets/killall"
	"goblin/pkg/core"
	"goblin/pkg/testutil"
)

func TestKillallInvalid(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "missing",
			Args:     []string{},
			WantCode: core.ExitUsage,
		},
		{
			Name:     "invalid_option",
			Args:     []string{"-Z"},
			WantCode: core.ExitUsage,
			WantErr:  "invalid option",
		},
	}
	testutil.RunAppletTests(t, killall.Run, tests)
}
