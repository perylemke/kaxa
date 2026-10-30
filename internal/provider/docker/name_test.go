package docker

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateName(t *testing.T) {
	t.Parallel()
	for _, ok := range []string{"a", testSandbox, "my-box-2", strings.Repeat("a", 63)} {
		if err := validateName(ok); err != nil {
			t.Errorf("validateName(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"", "-a", "a-", "A", "a_b", "a b", "a/b", strings.Repeat("a", 64)} {
		if err := validateName(bad); err == nil {
			t.Errorf("validateName(%q) must fail", bad)
		}
	}
}

func TestValidateTemplate(t *testing.T) {
	t.Parallel()
	for _, ok := range []string{"", testImage, "registry.example.com/team/img@sha256:abc", "docker/sandbox-templates:shell"} {
		if err := validateTemplate(ok); err != nil {
			t.Errorf("validateTemplate(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"-x", "a b", "a;b", "a/../b", "a\nb", strings.Repeat("a", 300)} {
		if err := validateTemplate(bad); err == nil {
			t.Errorf("validateTemplate(%q) must fail", bad)
		}
	}
}

func TestSetDefaultTemplateRejectsBadName(t *testing.T) {
	t.Parallel()
	if err := New().SetDefaultTemplate("a b"); err == nil {
		t.Fatal("expected an error")
	}
}

func TestResourceID(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{testRes: "abc", "abc": "abc", "": "", "a/b/c": "c"} {
		if got := resourceID(in); got != want {
			t.Errorf("resourceID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRedactedErrorUnwraps(t *testing.T) {
	t.Parallel()
	cause := errors.New("failed with " + testPAT)
	err := userErr(cause, "x", testPAT)
	if strings.Contains(err.Error(), testPAT) || !errors.Is(err, cause) {
		t.Fatalf("err = %v", err)
	}
	if got := userErr(nil, "x"); got != nil {
		t.Fatalf("userErr(nil) = %v", got)
	}
	plain := errors.New("plain")
	if got := userErr(plain, "x", testPAT); !errors.Is(got, plain) {
		t.Fatalf("an error with no secret must be returned as is, got %v", got)
	}
}
