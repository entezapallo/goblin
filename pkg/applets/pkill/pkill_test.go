package pkill_test

import (
	"testing"

	"goblin/pkg/applets/pkill"
	"goblin/pkg/core"
	"goblin/pkg/testutil"
)

func TestPkillInvalid(t *testing.T) {
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
	testutil.RunAppletTests(t, pkill.Run, tests)
}
