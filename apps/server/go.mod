module non24.app/server

go 1.26.0

require (
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2
	golang.org/x/crypto v0.48.0
	golang.org/x/sys v0.42.0
	modernc.org/sqlite v1.52.0
	non24.app/core v0.0.0
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/text v0.34.0 // indirect
	modernc.org/libc v1.72.3 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

replace non24.app/core => ../../core
