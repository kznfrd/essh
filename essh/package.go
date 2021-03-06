package essh

import (
	"fmt"
	"github.com/hashicorp/go-getter"
	"github.com/kohkimakimoto/essh/support/color"
	"github.com/yuin/gopher-lua"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type Package struct {
	// Name is url that is used as go-getter src.
	// examples:
	//   github.com/aaa/bbb
	//   git::github.com/aaa/bbb.git
	Name string
	// Value is a lua value that is returned when a module's 'index.lua' file is evaluated.
	Value lua.LValue
}

func NewPackage(name string) *Package {
	return &Package{
		Name: name,
	}
}

func (m *Package) Load(update bool) error {
	// If you usually use git with essh, you can set variable "GIT_SSH=essh".
	// But this setting cause a error in a module loading.
	// When we load a module, essh can git protocol, but essh hasn't generated ssh_config used by git command.
	gitssh := os.Getenv("GIT_SSH")
	if filepath.Base(gitssh) == "essh" {
		os.Setenv("GIT_SSH", "ssh")
		defer func() {
			os.Setenv("GIT_SSH", gitssh)
		}()
	}

	src := m.Name
	dst := m.Dir()

	if !update {
		if _, err := os.Stat(dst); err == nil {
			// If the directory already exists, then we're done since
			// we're not updating.
			return nil
		} else if !os.IsNotExist(err) {
			// If the error we got wasn't a file-not-exist error, then
			// something went wrong and we should report it.
			return fmt.Errorf("Error reading directory: %s", err)
		}
	}

	if debugFlag {
		fmt.Printf("[essh debug] module src '%s'\n", src)
	}

	if update {
		if _, err := os.Stat(dst); err == nil {
			fmt.Fprintf(os.Stdout, "This functionality deprecated! Updating package: '%s' (into %s)\n", color.FgYB(m.Name), color.FgBold(CurrentRegistry.DataDir))
		} else {
			fmt.Fprintf(os.Stdout, "This functionality deprecated! Installing package: '%s' (into %s)\n", color.FgYB(m.Name), color.FgBold(CurrentRegistry.DataDir))
		}
	} else {
		fmt.Fprintf(os.Stdout, "This functionality deprecated! Installing package: '%s' (into %s)\n", color.FgYB(m.Name), color.FgBold(CurrentRegistry.DataDir))
	}

	pwd, err := os.Getwd()
	if err != nil {
		return err
	}

	client := &getter.Client{
		Src:  src,
		Dst:  dst,
		Pwd:  pwd,
		Mode: getter.ClientModeDir,
	}
	if err := client.Get(); err != nil {
		return err
	}

	return nil
}

func (m *Package) IndexFile() string {
	return path.Join(m.Dir(), "index.lua")
}

func (m *Package) Dir() string {
	return path.Join(CurrentRegistry.PackagesDir(), m.Key())
}

func (m *Package) Key() string {
	return strings.Replace(strings.Replace(m.Name, "/", "-", -1), ":", "-", -1)
}
