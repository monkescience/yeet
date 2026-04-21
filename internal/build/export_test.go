package build

// SetForTest overrides the build metadata vars and returns a cleanup function
// that restores the previous values. Test-only.
func SetForTest(v, c, d string) func() {
	previousVersion, previousCommit, previousDate := version, commit, date
	version, commit, date = v, c, d

	return func() {
		version, commit, date = previousVersion, previousCommit, previousDate
	}
}
