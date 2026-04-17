// Package ui provides shared terminal styles for CLI output.
package ui

import (
	"charm.land/lipgloss/v2"
)

var (
	// Header is used for section titles.
	Header = lipgloss.NewStyle().Bold(true)

	// Label is used for left-column labels in key-value layouts.
	Label = lipgloss.NewStyle().Faint(true).Width(16) //nolint:mnd // label column width

	// Value is used for highlighted values.
	Value = lipgloss.NewStyle().Foreground(lipgloss.BrightGreen)

	// Faint is used for secondary information.
	Faint = lipgloss.NewStyle().Faint(true)

	// Separator renders a horizontal rule.
	Separator = "────────────────────────────────"
)
