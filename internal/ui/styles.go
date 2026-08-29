package ui

import (
	"charm.land/lipgloss/v2"
)

var (
	Header = lipgloss.NewStyle().Bold(true)

	Value = lipgloss.NewStyle().Foreground(lipgloss.BrightGreen)

	Faint = lipgloss.NewStyle().Faint(true)
)
