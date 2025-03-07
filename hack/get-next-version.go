// Copyright KubeArchive Authors
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Masterminds/semver/v3"
)

type flags struct {
	CurrentVersion   string
	ReleaseNotesFile string
}

var defaultValues = &flags{
	CurrentVersion:   "v0.0.0",
	ReleaseNotesFile: "./release-notes.json",
}

func main() {
	var flagValues flags
	flag.StringVar(&flagValues.CurrentVersion, "current-version", defaultValues.CurrentVersion, "Path to the file containing the version")
	flag.StringVar(&flagValues.ReleaseNotesFile, "release-notes-file", defaultValues.ReleaseNotesFile, "Path to the file containing the release notes in JSON format")
	flag.Parse()

	releaseNotesBytes, err := os.ReadFile(flagValues.ReleaseNotesFile)
	if err != nil {
		panic(fmt.Sprintf("Couldn't read '%s'", flagValues.ReleaseNotesFile))
	}

	var releaseNotes map[string]any
	if err = json.Unmarshal(releaseNotesBytes, &releaseNotes); err != nil {
		panic(err)
	}

	var collectedKinds string
	var errs error
	for _, pr := range releaseNotes {
		prMap := pr.(map[string]any)
		kinds, ok := prMap["kinds"]
		if !ok {
			errs = errors.Join(fmt.Errorf("PR '%v' does not have 'kind/*' labels, please add one.", prMap["pr_number"]), errs)
			continue
		}

		for _, kind := range kinds.([]any) {
			collectedKinds += fmt.Sprintf("%s ", kind.(string))
		}
	}

	if errs != nil {
		panic(fmt.Sprintf("some PRs do not have 'kind/*' labels:\n%s", errs.Error()))
	}

	fmt.Fprintf(os.Stderr, "DEBUG: Collected Kinds: %s\n", collectedKinds)

	version, err := semver.NewVersion(flagValues.CurrentVersion)
	if err != nil {
		panic(fmt.Sprintf("Version '%s' on '%s' is not valid, error :%s", version, flagValues.CurrentVersion, err))
	}

	if strings.Contains(collectedKinds, "breaking") {
		fmt.Printf("v%s\n", version.IncMajor())
		return
	}

	if strings.Contains(collectedKinds, "feature") {
		fmt.Printf("v%s\n", version.IncMinor())
		return
	}

	if strings.Contains(collectedKinds, "documentation") || strings.Contains(collectedKinds, "bug") {
		fmt.Printf("v%s\n", version.IncPatch())
		return
	}

	panic("No SemVer increase detected.")
}
