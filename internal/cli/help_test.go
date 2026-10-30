package cli_test

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/perylemke/kaxa/internal/cli"
	"github.com/perylemke/kaxa/internal/provider/e2b"
	"github.com/perylemke/kaxa/internal/provider/modal"
	"github.com/perylemke/kaxa/internal/provider/sprites"
	"github.com/perylemke/kaxa/internal/registry"
	"github.com/perylemke/kaxa/internal/sandbox"
)

const (
	sectionCommands = "Commands"
	flagsProvider   = "Provider flags"
	cmdSnapshot     = "snapshot"
	cmdVersion      = "version"
	flagAPIPort     = "--api-port"
	flagPublic      = "--public"
)

func allProvidersRegistry(t *testing.T) *registry.Registry {
	t.Helper()
	reg := registry.New()
	for _, p := range []sandbox.Provider{sprites.New(), modal.New(), e2b.New()} {
		if err := reg.Register(p); err != nil {
			t.Fatalf("register %s: %v", p.Name(), err)
		}
	}
	return reg
}

func helpOutput(t *testing.T, reg *registry.Registry, args ...string) string {
	t.Helper()
	cmd := cli.NewRootCmd(reg)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(new(bytes.Buffer))
	cmd.SetArgs(append(args, "--help"))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute %v: %v", args, err)
	}
	return out.String()
}

// helpSections returns the first word of each indented line under every
// unindented "Title:" heading.
func helpSections(help string) map[string][]string {
	sections := map[string][]string{}
	current := ""
	for line := range strings.SplitSeq(help, "\n") {
		switch {
		case line == "":
			current = ""
		case !strings.HasPrefix(line, " ") && strings.HasSuffix(line, ":"):
			current = strings.TrimSuffix(line, ":")
			sections[current] = []string{}
		case current != "" && strings.HasPrefix(line, "  "):
			sections[current] = append(sections[current], strings.Fields(line)[0])
		}
	}
	return sections
}

// helpBlock returns the lines under the heading title, up to the next blank line.
func helpBlock(help, title string) string {
	_, rest, ok := strings.Cut(help, "\n"+title+":\n")
	if !ok {
		return ""
	}
	block, _, _ := strings.Cut(rest, "\n\n")
	return block
}

func TestRootHelpListsEveryCommandInOneList(t *testing.T) {
	t.Parallel()

	help := helpOutput(t, allProvidersRegistry(t))
	sections := helpSections(help)

	want := []string{
		"completion", testCmdConsole, testCmdCreate, testCmdDestroy, testCmdExec, testCmdList, "mcp",
		testCmdRestart, cmdSnapshot, "sync", testCmdUpgrade, testCmdURL, cmdVersion,
	}
	if got := sections[sectionCommands]; !slices.Equal(got, want) {
		t.Errorf("Commands = %v, want %v\n%s", got, want, help)
	}
	for _, unwanted := range []string{"Available Commands:", "Additional Commands:", "Provider commands:", "Sprites:", "Modal:", "E2B:"} {
		if strings.Contains(help, unwanted) {
			t.Errorf("help should not contain %q:\n%s", unwanted, help)
		}
	}
}

func TestRootHelpIntroDoesNotNameProviders(t *testing.T) {
	t.Parallel()

	help := helpOutput(t, allProvidersRegistry(t))
	intro, _, _ := strings.Cut(help, "Usage:")
	if !strings.Contains(intro, "Supported providers: e2b, modal, sprites.") {
		t.Errorf("intro should list the registered providers:\n%s", intro)
	}
	if strings.Contains(intro, "Sprites or Modal") {
		t.Errorf("intro should not hardcode providers:\n%s", intro)
	}
}

func TestRootHelpListsEveryCommandForASingleProvider(t *testing.T) {
	t.Parallel()

	reg := registry.New()
	if err := reg.Register(modal.New()); err != nil {
		t.Fatalf("register: %v", err)
	}
	got := helpSections(helpOutput(t, reg))[sectionCommands]
	for _, name := range []string{testCmdRestart, testCmdURL, cmdSnapshot} {
		if !slices.Contains(got, name) {
			t.Errorf("Commands = %v, want %q listed", got, name)
		}
	}
}

func TestSubcommandKeepsPlainCommandList(t *testing.T) {
	t.Parallel()

	help := helpOutput(t, allProvidersRegistry(t), cmdSnapshot)
	sections := helpSections(help)

	want := []string{testCmdCreate, testCmdList, "restore", "delete"}
	got := sections["Available Commands"]
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Fatalf("snapshot commands = %v, want %v\n%s", got, want, help)
	}
}

func TestCreateHelpSplitsProviderFlags(t *testing.T) {
	t.Parallel()

	help := helpOutput(t, allProvidersRegistry(t), testCmdCreate)

	generic := helpBlock(help, "Flags")
	for _, want := range []string{"--sync", "--dir", testHelpFlag} {
		if !strings.Contains(generic, want) {
			t.Errorf("Flags should contain %q:\n%s", want, help)
		}
	}
	providerFlags := helpBlock(help, flagsProvider)
	for _, want := range []string{"--api ", "--api-cmd", "--api-env", flagAPIPort, flagPublic, "--running", "--template"} {
		if !strings.Contains(providerFlags, want) {
			t.Errorf("Provider flags should contain %q:\n%s", want, help)
		}
	}
	for _, unwanted := range []string{"--sync", "--dir", testHelpFlag, "--provider"} {
		if strings.Contains(providerFlags, unwanted) {
			t.Errorf("Provider flags should not contain %q:\n%s", unwanted, help)
		}
	}
	for _, unwanted := range []string{"--api", flagPublic, "--running", "--template"} {
		if strings.Contains(generic, unwanted) {
			t.Errorf("Flags should not contain %q:\n%s", unwanted, help)
		}
	}
	if !strings.Contains(helpBlock(help, "Global Flags"), "--provider") {
		t.Errorf("Global Flags should keep --provider:\n%s", help)
	}
}

func TestSnapshotRestoreHelpSplitsProviderFlags(t *testing.T) {
	t.Parallel()

	help := helpOutput(t, allProvidersRegistry(t), cmdSnapshot, "restore")

	generic := helpBlock(help, "Flags")
	if !strings.Contains(generic, "--force") || strings.Contains(generic, "--to") {
		t.Errorf("Flags should hold --force and not --to:\n%s", help)
	}
	providerFlags := helpBlock(help, flagsProvider)
	if !strings.Contains(providerFlags, "--to") || strings.Contains(providerFlags, "--force") {
		t.Errorf("Provider flags should hold --to and not --force:\n%s", help)
	}
}

func TestHelpOmitsProviderFlagsWhenAllAreGeneric(t *testing.T) {
	t.Parallel()

	for _, args := range [][]string{
		{testCmdDestroy},
		{"sync"},
		{cmdSnapshot, testCmdCreate},
		{cmdSnapshot, "delete"},
	} {
		help := helpOutput(t, allProvidersRegistry(t), args...)
		if strings.Contains(help, flagsProvider) {
			t.Errorf("%v help should not have %q:\n%s", args, flagsProvider, help)
		}
		if !strings.Contains(help, "\nFlags:\n") {
			t.Errorf("%v help should keep Flags:\n%s", args, help)
		}
	}
}

func TestProviderFlagDescriptionListsRegisteredProviders(t *testing.T) {
	t.Parallel()

	help := helpOutput(t, allProvidersRegistry(t), testCmdList)
	global := helpBlock(help, "Global Flags")
	if !strings.Contains(global, "e2b, modal, sprites") {
		t.Fatalf("--provider should list the registered providers:\n%s", help)
	}
}
