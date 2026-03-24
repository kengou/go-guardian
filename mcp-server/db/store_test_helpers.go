//go:build testing

package db

// SetLastRunForTest backdates the last_run timestamp for a scan type.
// This function exists only in test builds (//go:build testing).
// Production binaries never include this symbol.
func (s *Store) SetLastRunForTest(scanType, project, lastRun string) error {
	const q = `UPDATE scan_history SET last_run=? WHERE scan_type=? AND project=?`
	_, err := s.db.Exec(q, lastRun, scanType, project)
	return err
}
