BEGIN_COMMIT_OVERRIDE
feat(auth)!: replace session cookie format

Session cookies now use a keyed format.

BREAKING CHANGE: existing session cookies are invalid after upgrade
END_COMMIT_OVERRIDE