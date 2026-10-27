// Package project identifies a repository's build ecosystems from its root.
package project

import (
	"fmt"
	"os"
	"path/filepath"
)

// Kind is a project ecosystem detected at the repository root.
type Kind string

const (
	// KindGo is a Go module.
	KindGo Kind = "go"
	// KindRust is a Cargo project.
	KindRust Kind = "rust"
	// KindPython is a Python project installed with uv.
	KindPython Kind = "python"
	// KindNode is a Node.js project installed with npm.
	KindNode Kind = "node"
)

// Step is one ecosystem to prepare, with commands in order.
// Each command's first argument is the logical tool name.
type Step struct {
	Kind     Kind
	Commands [][]string
}

// Detect reports ecosystems present as files in the repository root.
// Go, Rust, Python, and Node are returned in that order. Nested directories are ignored.
func Detect(root string) ([]Step, error) {
	var steps []Step
	goMod, err := regularFile(root, "go.mod")
	if err != nil {
		return nil, err
	}
	if goMod {
		steps = append(steps, Step{Kind: KindGo, Commands: [][]string{{"go", "build", "./..."}}})
	}
	rust, err := rustStep(root)
	if err != nil {
		return nil, err
	}
	if rust != nil {
		steps = append(steps, *rust)
	}
	python, err := pythonStep(root)
	if err != nil {
		return nil, err
	}
	if python != nil {
		steps = append(steps, *python)
	}
	node, err := nodeStep(root)
	if err != nil {
		return nil, err
	}
	if node != nil {
		steps = append(steps, *node)
	}
	return steps, nil
}

func rustStep(root string) (*Step, error) {
	ok, err := regularFile(root, "Cargo.toml")
	if err != nil || !ok {
		return nil, err
	}
	argv := []string{"cargo", "build"}
	locked, err := regularFile(root, "Cargo.lock")
	if err != nil {
		return nil, err
	}
	if locked {
		argv = append(argv, "--locked")
	}
	return &Step{Kind: KindRust, Commands: [][]string{argv}}, nil
}

func pythonStep(root string) (*Step, error) {
	pyproject, err := regularFile(root, "pyproject.toml")
	if err != nil {
		return nil, err
	}
	uvLock, err := regularFile(root, "uv.lock")
	if err != nil {
		return nil, err
	}
	if pyproject || uvLock {
		return &Step{Kind: KindPython, Commands: [][]string{{"uv", "sync"}}}, nil
	}
	requirements, err := regularFile(root, "requirements.txt")
	if err != nil || !requirements {
		return nil, err
	}
	return &Step{Kind: KindPython, Commands: [][]string{
		{"uv", "venv", ".venv"},
		{"uv", "pip", "install", "--python", ".venv/bin/python", "-r", "requirements.txt"},
	}}, nil
}

func nodeStep(root string) (*Step, error) {
	pkg, err := regularFile(root, "package.json")
	if err != nil || !pkg {
		return nil, err
	}
	lock, err := regularFile(root, "package-lock.json")
	if err != nil {
		return nil, err
	}
	argv := []string{"npm", "install"}
	if lock {
		argv = []string{"npm", "ci"}
	}
	return &Step{Kind: KindNode, Commands: [][]string{argv}}, nil
}

func regularFile(root, name string) (bool, error) {
	info, err := os.Stat(filepath.Join(root, name))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat %s: %w", name, err)
	}
	return info.Mode().IsRegular(), nil
}
