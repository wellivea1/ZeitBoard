package localagent

import "non24.app/core/platform/privatefile"

// restrictToCurrentUser limits the descriptor to the user running this process.
//
// The implementation used to live here, added by ADR-0028 because os.Chmod's
// Unix spelling does not restrict anything on Windows. It now lives in
// core/platform/privatefile, because the descriptor was never the only private
// file: the database holding every recorded sleep had the same problem and none
// of the protection.
func restrictToCurrentUser(path string) error { return privatefile.Restrict(path) }
