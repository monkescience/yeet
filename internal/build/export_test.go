package build

func SetForTest(v, c, d string) func() {
	previousVersion, previousCommit, previousDate := version, commit, date
	version, commit, date = v, c, d

	return func() {
		version, commit, date = previousVersion, previousCommit, previousDate
	}
}
