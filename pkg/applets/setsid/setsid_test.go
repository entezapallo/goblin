package setsid_test

import (
	"testing"

	"goblin/pkg/applets/setsid"
	"goblin/pkg/core"
	"goblin/pkg/testutil"
)

func TestSetsid(t *testing.T) {
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
	testutil.RunAppletTests(t, setsid.Run, tests)
}
