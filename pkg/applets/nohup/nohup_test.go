package nohup_test

import (
	"testing"

	"goblin/pkg/applets/nohup"
	"goblin/pkg/core"
	"goblin/pkg/testutil"
)

func TestNohup(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "missing",
			Args:     []string{},
			WantCode: core.ExitUsage,
		},
		{
			Name:     "basic",
			Args:     []string{"echo", "ok"},
			WantCode: core.ExitSuccess,
		},
	}
	testutil.RunAppletTests(t, nohup.Run, tests)
}
