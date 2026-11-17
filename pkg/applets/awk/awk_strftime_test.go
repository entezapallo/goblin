package awk_test

import (
	"testing"

	"goblin/pkg/applets/awk"
	"goblin/pkg/core"
	"goblin/pkg/testutil"
)

func TestStrftime(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "epoch",
			Args:     []string{"BEGIN { print strftime(\"%s\", 0) }"},
			WantCode: core.ExitSuccess,
			WantOut:  "0\n",
		},
		{
			Name:     "day_of_year",
			Args:     []string{"BEGIN { print strftime(\"%j\", 0) }"},
			WantCode: core.ExitSuccess,
			WantOut:  "001\n",
		},
		{
			Name:     "weekday_u_w",
			Args:     []string{"BEGIN { print strftime(\"%u %w\", 0) }"},
			WantCode: core.ExitSuccess,
			WantOut:  "4 4\n",
		},
	}
	testutil.RunAppletTests(t, awk.Run, tests)
}
