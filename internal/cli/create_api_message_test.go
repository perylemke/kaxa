package cli

import (
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/sandbox"
)

func TestCreatedMessageNamesTheAccessOfTheProvider(t *testing.T) {
	t.Parallel()
	const apiURL = "https://8000-abc.example.test"
	tests := []struct {
		name     string
		provider string
		auth     string
		req      createRequest
		want     string
		notWant  string
	}{
		{name: "sprites", provider: spritesProviderName, req: createRequest{API: true, APIPort: 8000}, want: "requires a Sprites account"},
		{name: "sprites made public", provider: spritesProviderName, req: createRequest{API: true, APIPort: 8000, Public: true}, want: "access is public", notWant: "Sprites account"},
		{name: "e2b is open", provider: "e2b", auth: sandbox.URLAuthPublic, req: createRequest{API: true, APIPort: 8000}, want: "access is public", notWant: "Sprites"},
		{name: "docker says nothing", provider: "docker", req: createRequest{API: true, APIPort: 8000}, notWant: "Sprites"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := createdMessage("box", tt.provider, sandbox.Detail{URL: apiURL, Auth: tt.auth}, tt.req)
			if !strings.Contains(got, apiURL) || !strings.Contains(got, "forwards to port 8000") {
				t.Fatalf("message = %q", got)
			}
			if tt.want != "" && !strings.Contains(got, tt.want) {
				t.Fatalf("message = %q, want %q", got, tt.want)
			}
			if tt.notWant != "" && strings.Contains(got, tt.notWant) {
				t.Fatalf("message = %q, must not mention %q", got, tt.notWant)
			}
		})
	}
}
