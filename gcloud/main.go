/*
Copyright 2016 Google Inc. All rights reserved.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

/*
// uncomment for completion debugging
var sl *log.Logger
func init() {
	fout, _ := os.Create("secret_log")
	sl = log.New(fout, "", 0)
}
*/

// Relevant environment variables, for easy discovery.
const (
	// The directory containing pincloud config information.
	// Defaults to "$HOME/.config/pincloud".
	configDirEnv = "PINCLOUD_CONFIG_DIR"
	// The file containing command version pins.
	// Defaults to "$PINCLOUD_CONFIG_DIR/pins.cfg".
	configEnv = "PINCLOUD_CONFIG"
	// The directory containing sdk versinos.
	// Defaults to "$PINCLOUD_CONFIG_DIR/versions".
	configVersionsDirEnv = "PINCLOUD_CONFIG_VERSIONS_DIR"
)

func main() {

	// If it's a special pincloud management command, don't forward to gcloud.
	if pincloudCommand() {
		return
	}

	fin, err := os.Open(getPinsPath())
	if err != nil {
		log.Fatalf("Could not open pin config: %v.", err)
	}
	plist, err := loadPins(fin)
	if err != nil {
		log.Fatalf("Could not load pins: %v.", err)
	}

	commandArgs := os.Args
	if compLine := os.Getenv("COMP_LINE"); compLine != "" {
		shlexed, err := shlex(compLine)
		if err == nil {
			commandArgs = shlexed
		}
	}

	args, err := plist.mapCommand(commandArgs)
	if err != nil {
		log.Fatalf("Could not map command: %v.", err)
	}

	if info, err := os.Stat(args[0]); err != nil {
		log.Fatalf("Invalid pin: %q does not exist.", args[0])
	} else if info.IsDir() {
		log.Fatalf("Invalid pin: %q is a directory.", args[0])
	}

	log.Printf("Using %q", args[0])

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		if err, ok := err.(*exec.ExitError); ok {
			if status, ok := err.Sys().(syscall.WaitStatus); ok {
				os.Exit(status.ExitStatus())
			}
		}
		// Oh well, use 1.
		os.Exit(1)
	}
}

func pincloudCommand() bool {
	if len(os.Args) == 1 || os.Args[1] != "pincloud" {
		return false
	}

	if len(os.Args) != 4 {
		log.Fatalf("Usage: %s pincloud {install,remove} VERSION", os.Args[0])
	}

	version := os.Args[3]
	switch os.Args[2] {
	case "install":
		versionsDir := getVersionsDirectory()
		versionDir := filepath.Join(versionsDir, version)
		if _, err := os.Stat(versionDir); err == nil {
			log.Fatalf("Something is in the way at %q.", versionDir)
		}
		sdkDir, ok := getDefaultSDK()
		if !ok {
			log.Fatalf("Could not find the default SDK to clone.")
		}

		if err := os.MkdirAll(versionsDir, 0755); err != nil {
			log.Fatalf("Could not create %q: %v", versionsDir, err)
		}
		log.Print("Cloning the default SDK.")
		if err := exec.Command("cp", "-r", sdkDir, versionDir).Run(); err != nil {
			log.Fatalf("Could not clone default SDK: %v", err)
		}
		log.Printf("Updating the cloned SDK to version %s.", version)
		updateCmd := exec.Command(filepath.Join(versionDir, "bin", "gcloud"), "components", "update", "-q", "--version", version)
		updateCmd.Stdout = os.Stdout
		updateCmd.Stderr = os.Stderr
		if err := updateCmd.Run(); err != nil {
			log.Fatalf("Could not update cloned SDK: %v", err)
		}
		log.Print("Install complete. Ignore the warnings about old versions of the tools.")
	case "remove":
		if err := os.RemoveAll(filepath.Join(getVersionsDirectory(), version)); err != nil {
			log.Fatalf("Error removing version %q: %v", version, err)
		}
	default:
		log.Fatalf("Usage: %s pincloud {install,remove} VERSION", os.Args[0])
	}

	return true
}

func getConfigDirectory() string {
	if dir := os.Getenv(configDirEnv); dir != "" {
		return dir
	}
	home := os.Getenv("HOME")
	return filepath.Join(home, ".config", "pincloud")
}

func getVersionsDirectory() string {
	if dir := os.Getenv(configVersionsDirEnv); dir != "" {
		return dir
	}
	return filepath.Join(getConfigDirectory(), "versions")
}

func getPinsPath() string {
	if p := os.Getenv(configEnv); p != "" {
		return p
	}
	return filepath.Join(getConfigDirectory(), "pins.cfg")
}

func shlex(s string) ([]string, error) {
	// TODO: real shlexing.
	return strings.Split(strings.TrimSpace(s), " "), nil
}

type Pin struct {
	Pattern []string
	Args    []string
}

type PinList []Pin

func loadPins(r io.Reader) (PinList, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("could not read pin data: %v", err)
	}
	lines := strings.Split(string(data), "\n")
	var plist PinList
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if len(l) == 0 || l[0] == '#' {
			continue
		}
		tokens := strings.SplitN(l, ":", 2)
		if len(tokens) != 2 {
			return nil, fmt.Errorf("not of form 'PATTERN:ARGS': %q", l)
		}

		var p Pin
		var err error

		p.Pattern, err = shlex(tokens[0])
		if err != nil {
			return nil, fmt.Errorf("could not shlex %q: %v", tokens[0], err)
		}
		if len(p.Pattern) == 0 {
			return nil, fmt.Errorf("zero-len pattern in %q", l)
		}
		if p.Pattern[0] != "gcloud" {
			return nil, fmt.Errorf("first token in pattern must be 'gcloud', not %q", p.Pattern[0])
		}

		p.Args, err = shlex(tokens[1])
		if err != nil {
			return nil, fmt.Errorf("could not shlex %q: %v", tokens[1], err)
		}
		if len(p.Args) == 0 {
			return nil, fmt.Errorf("zero-len args in %q", l)
		}
		// p.Args[0] is the sdk version to use. If it's not absolute, it's
		// located in the versions directory.
		if !filepath.IsAbs(p.Args[0]) {
			p.Args[0] = filepath.Join(getVersionsDirectory(), p.Args[0])
		}
		// We actually want the bin/gcloud from within the directory indicated.
		p.Args[0] = filepath.Join(p.Args[0], "bin", "gcloud")

		plist = append(plist, p)
	}
	return plist, nil
}

func (plist PinList) mapCommand(args []string) ([]string, error) {
	// Skip non-positionals.
	var positionals []string
	for _, arg := range args {
		if arg[0] == '-' {
			continue
		}
		positionals = append(positionals, arg)
	}

	// Find the first pattern that prefix-matches the positionals.

	var partialPatternMatch Pin

plist:
	for _, p := range plist {
		pat := p.Pattern
		// Cannot be a prefix if positionals is shorter.
		if len(pat) > len(positionals) {
			continue
		}

		// Copy the slice pointer so that we can trim off the front.
		checkArgs := positionals

		// The first token always matches, since it's 'gcloud' in the pattern
		// and this command in args[0], so we skip them.
		pat = pat[1:]
		checkArgs = checkArgs[1:]

		for len(pat) > 0 {
			if pat[0] != checkArgs[0] {
				// If this is the last word and it's a prefix match, find the
				// closest thing for completion and help messages.
				if len(pat) == 1 && strings.HasPrefix(pat[0], checkArgs[0]) {
					partialPatternMatch = p
				}
				// Mismatch - try the next pattern.
				continue plist
			}
			// So far so good, move to the next token.
			pat = pat[1:]
			checkArgs = checkArgs[1:]
		}

		// Prefix match, use this pin.
		pinnedArgs := append([]string{}, p.Args...)
		pinnedArgs = append(pinnedArgs, args[1:]...)
		prepareEnvForCompletion(p.Args)
		return pinnedArgs, nil
	}

	if partialPatternMatch.Pattern != nil {
		// partial match, we still use it.
		pinnedArgs := append([]string{}, partialPatternMatch.Args...)
		pinnedArgs = append(pinnedArgs, args[1:]...)
		prepareEnvForCompletion(partialPatternMatch.Args)
		return pinnedArgs, nil
	}

	// No patterns matched, so use the default gcloud.
	gcloud, ok := getDefaultGcloud()
	if !ok {
		return nil, fmt.Errorf("no patterns matched, and no gcloud on path")
	}
	return append([]string{gcloud}, args[1:]...), nil
}

// Get the first gcloud on the path that isn't this binary.
// The heuristic is that the first line of the file is "#!/bin/sh".
func getDefaultGcloud() (string, bool) {
	symbol := "#!/bin/sh"

	pathEnv := os.Getenv("PATH")
	pathDirs := filepath.SplitList(pathEnv)
	for _, pd := range pathDirs {
		candidateGcloud := filepath.Join(pd, "gcloud")
		cin, err := os.Open(candidateGcloud)
		if err != nil {
			log.Print("Could not open candidate gcloud %q: %v", candidateGcloud, err)
			continue
		}
		buf := make([]byte, len(symbol))
		n, err := io.ReadFull(cin, buf)
		if err != nil || n != len(symbol) {
			// probably EOF from it being too short
			continue
		}
		if string(buf) == symbol {
			return candidateGcloud, true
		}
	}
	return "", false
}

func getDefaultSDK() (string, bool) {
	defaultGcloud, ok := getDefaultGcloud()
	if !ok {
		return "", false
	}
	cmd := exec.Command(defaultGcloud, "info", "--format=value(installation.sdk_root)")
	data, err := cmd.Output()
	if err != nil {
		log.Printf("Problem running default gcloud to find default sdk: %v", err)
		return "", false
	}
	return strings.TrimSpace(string(data)), true
}

func prepareEnvForCompletion(args []string) {
	compLine := os.Getenv("COMP_LINE")
	if compLine == "" {
		// COMP_LINE not set, must not be doing completion.
		return
	}
	point, err := strconv.Atoi(os.Getenv("COMP_POINT"))
	if err != nil {
		return
	}

	if len(args) == 1 {
		// No special preparation is needed unless the pin adds a release track.
		return
	}

	words := strings.SplitN(compLine, " ", 2)
	if len(words) == 0 {
		return
	}

	oldLen := len(compLine)

	newWords := append([]string{words[0]}, args[1:]...)
	newWords = append(newWords, words[1:]...)
	compLine = strings.Join(newWords, " ")

	newLen := len(compLine)
	point += newLen - oldLen
	os.Setenv("COMP_LINE", compLine)
	os.Setenv("COMP_POINT", fmt.Sprint(point))
}
