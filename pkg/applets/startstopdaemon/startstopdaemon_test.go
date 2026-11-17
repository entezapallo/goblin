package startstopdaemon_test

import (
	"testing"

	"goblin/pkg/applets/startstopdaemon"
	"goblin/pkg/core"
	"goblin/pkg/testutil"
)

func TestStartStopDaemon(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "missing",
			Args:     []string{},
			WantCode: core.ExitUsage,
		},
		{
			Name:       "basic",
			Args:       []string{"--start", "--exec", "echo", "--", "ok"},
			WantCode:   core.ExitSuccess,
			WantOutSub: "\n",
		},
	}
	testutil.RunAppletTests(t, startstopdaemon.Run, tests)
}
