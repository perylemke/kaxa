package cli_test

import (
	"testing"

	"github.com/perylemke/sbox/internal/usererr"
)

func TestArgErrorsExplainUsage(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
		hint string
	}{
		{
			name: "create extra",
			args: []string{testCmdCreate, testExtraName, testExtraName2},
			want: "create accepts one sandbox name",
			hint: "sbox create [name]",
		},
		{
			name: "new extra",
			args: []string{"new", testExtraName, testExtraName2},
			want: "create accepts one sandbox name",
			hint: "sbox create [name]",
		},
		{
			name: "destroy extra",
			args: []string{testCmdDestroy, testExtraName, testExtraName2},
			want: "destroy accepts one sandbox name",
			hint: "sbox destroy [name]",
		},
		{
			name: "rm extra",
			args: []string{"rm", testExtraName, testExtraName2},
			want: "destroy accepts one sandbox name",
			hint: "sbox destroy [name]",
		},
		{
			name: "destroy all with name",
			args: []string{testCmdDestroy, testFlagAll, testExtraName},
			want: "destroy --all does not take a sandbox name",
			hint: "sbox destroy --all",
		},
		{
			name: "restart extra",
			args: []string{testCmdRestart, testExtraName, testExtraName2},
			want: "restart accepts one sandbox name",
			hint: "sbox restart [name]",
		},
		{
			name: "upgrade extra",
			args: []string{testCmdUpgrade, testExtraName, testExtraName2},
			want: "upgrade accepts one sandbox name",
			hint: "sbox upgrade [name]",
		},
		{
			name: "url missing",
			args: []string{testCmdURL},
			want: "url requires a sandbox name and an auth mode",
			hint: "sbox url <name> public|sprite",
		},
		{
			name: "url name only",
			args: []string{testCmdURL, testArgBox},
			want: "url requires an auth mode",
			hint: "sbox url <name> public|sprite",
		},
		{
			name: "url extra",
			args: []string{testCmdURL, testArgBox, "public", "extra"},
			want: "url accepts a sandbox name and an auth mode",
			hint: "sbox url <name> public|sprite",
		},
		{
			name: "list extra",
			args: []string{testCmdList, testArgBox},
			want: "list does not take arguments",
			hint: "sbox list",
		},
		{
			name: "ls extra",
			args: []string{"ls", testArgBox},
			want: "list does not take arguments",
			hint: "sbox list",
		},
		{
			name: "version extra",
			args: []string{"version", testArgBox},
			want: "version does not take arguments",
			hint: "sbox version",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := runSpritesCmd(t, tt.args...)
			ue, ok := usererr.As(err)
			if !ok || ue.Message != tt.want || ue.Hint != tt.hint {
				t.Fatalf("err = %#v", err)
			}
		})
	}
}
