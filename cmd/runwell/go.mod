module github.com/Martin-Business-Consultants/runwellgo/v3/cmd/runwell

go 1.26.0

toolchain go1.26.9

require (
	github.com/Martin-Business-Consultants/runwellgo-ai v1.4.0
	github.com/Martin-Business-Consultants/runwellgo-chat v1.1.0
	github.com/Martin-Business-Consultants/runwellgo-my-day v0.1.0
	github.com/Martin-Business-Consultants/runwellgo-plugin-template v1.0.0
	github.com/Martin-Business-Consultants/runwellgo-reports v1.1.0
	github.com/Martin-Business-Consultants/runwellgo-runners v1.0.0
	github.com/Martin-Business-Consultants/runwellgo-time-tracking v0.1.0
	github.com/Martin-Business-Consultants/runwellgo-websites v0.4.0
	github.com/Martin-Business-Consultants/runwellgo/v3 v3.0.2-0.20261019030058-311cea1a14b6
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/net v0.61.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	modernc.org/libc v1.77.1 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
	modernc.org/sqlite v1.60.1 // indirect
)

// The binary is always built with this repository's own core.
replace github.com/Martin-Business-Consultants/runwellgo/v3 => ../..
