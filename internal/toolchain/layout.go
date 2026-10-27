package toolchain

import (
	"fmt"

	"github.com/perylemke/kaxa/internal/project"
)

// Layout is the home directory and user a toolchain is installed for. Tools go
// under <Home>/.local and <Home>/.cargo, so no root access is needed.
type Layout struct {
	Home string
	User string
}

func (l Layout) localPrefix() string   { return l.Home + "/.local" }
func (l Layout) localBin() string      { return l.localPrefix() + "/bin" }
func (l Layout) configDir() string     { return l.Home + "/.config/kaxa" }
func (l Layout) pathFile() string      { return l.configDir() + "/path.sh" }
func (l Layout) bashrcFile() string    { return l.Home + "/.bashrc" }
func (l Layout) goBin() string         { return l.localPrefix() + "/go/bin/go" }
func (l Layout) cargoBin() string      { return l.Home + "/.cargo/bin/cargo" }
func (l Layout) uvBin() string         { return l.localBin() + "/uv" }
func (l Layout) npmBin() string        { return l.localBin() + "/npm" }
func (l Layout) nodePrefix() string    { return l.localPrefix() + "/node" }
func (l Layout) rubyPrefix() string    { return l.localPrefix() + "/ruby" }
func (l Layout) bundleBin() string     { return l.localBin() + "/bundle" }
func (l Layout) jdkPrefix() string     { return l.localPrefix() + "/jdk" }
func (l Layout) javaBin() string       { return l.localBin() + "/java" }
func (l Layout) clojurePrefix() string { return l.localPrefix() + "/lib/clojure" }
func (l Layout) clojureBin() string    { return l.localBin() + "/clojure" }

// Env is the environment that finds the installed tools: HOME, USER and PATH.
func (l Layout) Env() []string {
	return []string{
		"HOME=" + l.Home,
		"USER=" + l.User,
		"PATH=" + l.localBin() + ":" + l.localPrefix() + "/go/bin:" + l.Home + "/.cargo/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
	}
}

func (l Layout) toolPaths(kind project.Kind) (probe []string, bin string, err error) {
	switch kind {
	case project.KindGo:
		return []string{"go", "version"}, l.goBin(), nil
	case project.KindRust:
		return []string{"cargo", argvVersion}, l.cargoBin(), nil
	case project.KindPython:
		return []string{"uv", argvVersion}, l.uvBin(), nil
	case project.KindNode:
		return []string{"npm", argvVersion}, l.npmBin(), nil
	case project.KindRuby:
		return []string{"bundle", argvVersion}, l.bundleBin(), nil
	case project.KindClojure:
		return []string{"clojure", argvVersion}, l.clojureBin(), nil
	default:
		return nil, "", fmt.Errorf("unknown project kind %q", kind)
	}
}
