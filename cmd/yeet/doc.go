// Yeet automates releases from conventional commits. It analyzes commit history
// to determine the next version, generates changelogs, and creates release
// PRs/MRs on GitHub, GitLab, or Azure DevOps. On the default branch it finalizes
// merged release PRs/MRs by creating the provider release.
//
// Usage:
//
//	yeet [command]
//
// The main commands are release, init, and version. Run "yeet --help" for the
// full list of commands and flags, or see the README at
// https://github.com/monkescience/yeet.
package main
