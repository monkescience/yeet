package ui

import (
	"charm.land/lipgloss/v2"
)

var (
	Header = lipgloss.NewStyle().Bold(true)

	Label = lipgloss.NewStyle().Faint(true).Width(16) //nolint:mnd // label column width

	Value = lipgloss.NewStyle().Foreground(lipgloss.BrightGreen)

	Faint = lipgloss.NewStyle().Faint(true)

	Separator = "────────────────────────────────"
)
