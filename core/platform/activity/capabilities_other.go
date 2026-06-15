//go:build !windows

package activity

import "non24.app/core/ingest"

func platformCapabilities() ingest.Capabilities {
	return ingest.Capabilities{}
}
