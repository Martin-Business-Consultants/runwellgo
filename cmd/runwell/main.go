// runwell is the official Runwell binary: the core (runwell.Main) with the
// official plugins. A Runwell of your own is a main like this one with the
// plugins you choose (docs/plugins.md).
package main

import (
	"os"

	ai "github.com/Martin-Business-Consultants/runwellgo-ai"
	chat "github.com/Martin-Business-Consultants/runwellgo-chat"
	myday "github.com/Martin-Business-Consultants/runwellgo-my-day"
	hello "github.com/Martin-Business-Consultants/runwellgo-plugin-template"
	reports "github.com/Martin-Business-Consultants/runwellgo-reports"
	runners "github.com/Martin-Business-Consultants/runwellgo-runners"
	timetracking "github.com/Martin-Business-Consultants/runwellgo-time-tracking"
	websitechecklist "github.com/Martin-Business-Consultants/runwellgo-websites"
	"github.com/Martin-Business-Consultants/runwellgo/v3/runwell"
)

// version is this build's: set from the release tag (make dist VERSION=…),
// or the VERSION file (the Dockerfile).
var version = "dev"

func main() {
	os.Exit(runwell.Main(version,
		hello.Module(), timetracking.Module(), myday.Module(), websitechecklist.Module(),
		ai.Module(), chat.Module(), reports.Module(), runners.Module(),
	))
}
