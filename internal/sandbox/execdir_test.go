package sandbox

import "testing"

func TestResolveExecDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		requested string
		fallback  string
		want      string
		wantErr   bool
	}{
		{name: "fallback", fallback: "/home/sprite/workspace", want: "/home/sprite/workspace"},
		{name: "empty", want: ""},
		{name: "absolute", requested: "/home/sprite/workspace/app/", want: "/home/sprite/workspace/app"},
		{name: "folded", requested: "/home/sprite/workspace/app/../app", want: "/home/sprite/workspace/app"},
		{name: "relative", requested: "app", wantErr: true},
		{name: "nul", requested: "/tmp/\x00app", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ResolveExecDir(tt.requested, tt.fallback)
			if tt.wantErr {
				if err == nil {
					t.Fatal("accepted")
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("ResolveExecDir(%q, %q) = %q, %v", tt.requested, tt.fallback, got, err)
			}
		})
	}
}
