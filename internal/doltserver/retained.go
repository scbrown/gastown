package doltserver

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RetainedDatabasesLog is the town-relative path of the attribution log.
const RetainedDatabasesLog = "logs/retained-databases.jsonl"

// RetainedDatabase records a database that Gas Town declined to remove because
// it lives on a shared remote Dolt server rather than on this host.
//
// The point of this record is ATTRIBUTION, not bookkeeping (aegis-i08ls).
// Removing a rig — or cleaning up after one — is the LAST moment at which the
// mapping from database name to owning rig still exists anywhere. Once the
// rig's metadata is gone, nothing on this host can say what
// `beads_ma` or `fixdepkeys_64c9a5bd939e` belonged to, which is exactly why
// orphan-cleanup tooling ends up guessing from name patterns — and guessing is
// what proposed dropping a live service database in aegis-hphtm.
//
// So the retained database is written down while it is still knowable. An
// orphan you can name and attribute is a housekeeping item on a list; an
// unattributable one is a standing invitation to guess.
type RetainedDatabase struct {
	// Database is the name on the server.
	Database string `json:"database"`

	// Server is the Dolt server it remains on ("host:port").
	Server string `json:"server"`

	// Rig is the rig that owned it, and Prefix the beads prefix it was created
	// from. These are the fields that cannot be recovered later.
	Rig    string `json:"rig,omitempty"`
	Prefix string `json:"prefix,omitempty"`

	// Reason says why it was left behind, in the words of the caller.
	Reason string `json:"reason"`

	// Actor is whoever ran the command. Every agent on this host shares one
	// unix user, so USER alone cannot tell them apart — GT_ROLE/GT_AGENT can.
	Actor string `json:"actor"`

	// At is when it was retained.
	At time.Time `json:"at"`
}

// currentActor identifies who is running, preferring the Gas Town role over the
// unix user because every agent on a host shares that user.
func currentActor() string {
	for _, key := range []string{"GT_ROLE", "GT_AGENT", "USER"} {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return "unknown"
}

// RecordRetainedDatabase appends rec to the town's attribution log and returns
// the log path. Actor and At are filled in when unset.
//
// Never returns an error that should fail the caller's operation: leaving a
// database behind is already the degraded path, and failing a rig removal
// because a log file could not be written would trade a disk-space problem for
// a broken command. Callers report the error and carry on.
func RecordRetainedDatabase(townRoot string, rec RetainedDatabase) (string, error) {
	if rec.Actor == "" {
		rec.Actor = currentActor()
	}
	if rec.At.IsZero() {
		rec.At = time.Now()
	}

	logPath := filepath.Join(townRoot, RetainedDatabasesLog)
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return logPath, fmt.Errorf("creating log directory: %w", err)
	}

	line, err := json.Marshal(rec)
	if err != nil {
		return logPath, fmt.Errorf("encoding record: %w", err)
	}

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return logPath, fmt.Errorf("opening log: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(line, '\n')); err != nil {
		return logPath, fmt.Errorf("writing record: %w", err)
	}
	return logPath, nil
}

// ReportRetainedDatabase writes the attribution record and prints a report
// naming the database and the server it remains on.
//
// This is a REPORT, not a refusal (aegis-i08ls). The operation the caller was
// performing has succeeded; a database was left behind on a server this town
// does not own, and saying so is the whole job. Gas Town deliberately does NOT
// issue DROP DATABASE against a shared remote server — that server is used by
// other towns, and one town's bookkeeping is not authority to destroy data on
// it. Do not "fix" this by adding an automatic drop.
func ReportRetainedDatabase(townRoot string, rec RetainedDatabase) {
	logPath, err := RecordRetainedDatabase(townRoot, rec)

	fmt.Fprintf(os.Stderr, "Note: database %q was NOT removed — it lives on %s, not on this host.\n",
		rec.Database, rec.Server)
	if rec.Rig != "" {
		fmt.Fprintf(os.Stderr, "      It belonged to rig %q. That attribution is recorded now because\n", rec.Rig)
		fmt.Fprintf(os.Stderr, "      after this point nothing here can reconstruct it.\n")
	}
	if err != nil {
		// The log is the durable half; if it failed, the stderr line above is
		// the ONLY record, so say plainly that it is.
		fmt.Fprintf(os.Stderr, "      WARNING: could not write %s: %v\n", logPath, err)
		fmt.Fprintf(os.Stderr, "      This message is now the only record of it.\n")
		return
	}
	fmt.Fprintf(os.Stderr, "      Recorded in %s\n", logPath)
}
