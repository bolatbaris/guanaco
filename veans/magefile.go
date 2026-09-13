// Vikunja is a to-do list application to facilitate your life.
// Copyright 2018-present Vikunja and contributors. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

//go:build mage

// Mage targets for the veans CLI: dev build, tests, lint.
//
// Release tooling (xgo cross-compile, packaging, nfpm templating) lives in
// the centralized build/ module — run "mage release:build veans" from there.
package main

import (
	"os"
	"os/exec"
	"runtime"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

// Build compiles the veans binary into ./veans (or ./veans.exe on Windows).
func Build() error {
	out := "./veans"
	if runtime.GOOS == "windows" {
		out = "./veans.exe"
	}
	return sh.RunV("go", "build", "-o", out, "./cmd/veans")
}

// Clean removes built artifacts.
func Clean() error {
	for _, p := range []string{"./veans", "./veans.exe"} {
		if _, err := os.Stat(p); err == nil {
			if err := os.Remove(p); err != nil {
				return err
			}
		}
	}
	return nil
}

// Fmt runs goimports across the module.
func Fmt() error {
	return sh.RunV("go", "fmt", "./...")
}

// Lint namespace.
type Lint mg.Namespace

// All runs golangci-lint over the module.
func (Lint) All() error {
	if _, err := exec.LookPath("golangci-lint"); err != nil {
		return fmt.Errorf("golangci-lint not installed: %w", err)
	}
	return sh.RunV("golangci-lint", "run", "./...")
}

// Fix runs golangci-lint with --fix.
func (Lint) Fix() error {
	if _, err := exec.LookPath("golangci-lint"); err != nil {
		return fmt.Errorf("golangci-lint not installed: %w", err)
	}
	return sh.RunV("golangci-lint", "run", "--fix", "./...")
}

// Aliases keeps the commonly used targets short.
var Aliases = map[string]any{
	"lint":     Lint.All,
	"lint:fix": Lint.Fix,
}
