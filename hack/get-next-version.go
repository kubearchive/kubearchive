package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/Masterminds/semver/v3"
)

type flags struct {
	CurrentVersion   string
	ReleaseNotesFile string
	HelmChart        bool
}

var defaultValues = &flags{
	CurrentVersion:   "v0.0.0",
	ReleaseNotesFile: "./release-notes.json",
	HelmChart:        false,
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
	if err := json.Unmarshal(releaseNotesBytes, &releaseNotes); err != nil {
		panic(err)
	}

	var collectedKinds string
	for _, pr := range releaseNotes {
		kinds := pr.(map[string]any)["kinds"].([]any)
		for _, kind := range kinds {
			collectedKinds += fmt.Sprintf("%s ", kind.(string))
		}
	}

	fmt.Fprintf(os.Stderr, "DEBUG: Collected Kinds: %s\n", collectedKinds)

	version, err := semver.NewVersion(flagValues.CurrentVersion)
	if err != nil {
		panic(fmt.Sprintf("Version '%s' on '%s' is not valid.", version, flagValues.CurrentVersion))
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
