package commands

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"

	"charm.land/glamour/v2"
	glamouransi "charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	terminalansi "github.com/charmbracelet/x/ansi"
	"github.com/monkescience/yeet/internal/release"
	"github.com/monkescience/yeet/internal/ui"
)

const dryRunPreviewWidth = 80

func printDryRun(w io.Writer, result *release.Result) error {
	output, err := formatDryRun(result)
	if err != nil {
		return err
	}

	_, err = io.WriteString(w, output)
	if err != nil {
		return fmt.Errorf("write dry-run output: %w", err)
	}

	return nil
}

func formatDryRun(result *release.Result) (string, error) {
	var output bytes.Buffer

	_, _ = fmt.Fprintln(&output)
	_, _ = fmt.Fprintln(&output, ui.Header.Render("Dry Run"))
	_, _ = fmt.Fprintln(&output, ui.Faint.Render("No changes will be made."))

	if len(result.Plans) == 0 {
		_, _ = fmt.Fprintln(&output)
		_, _ = fmt.Fprintln(&output, ui.Faint.Render("No changed targets."))
		_, _ = fmt.Fprintln(&output)
		printDryRunSummary(&output, 0, 0, 0)

		return output.String(), nil
	}

	_, _ = fmt.Fprintln(&output)
	_, _ = fmt.Fprintln(&output, ui.Header.Render("Release Plan"))
	_, _ = fmt.Fprintln(&output)

	commitCount := 0

	for i, plan := range result.Plans {
		if i > 0 {
			_, _ = fmt.Fprintln(&output)
		}

		printDryRunTarget(&output, plan)
		commitCount += plan.CommitCount
	}

	pullRequestCount := 0
	if result.Text != nil {
		pullRequestCount = 1
		options := result.Text.PROptions

		_, _ = fmt.Fprintln(&output)
		_, _ = fmt.Fprintln(&output, ui.Header.Render("Pull Request"))
		_, _ = fmt.Fprintln(&output)
		printDryRunField(&output, "Action", "create or update")
		printDryRunField(&output, "Title", options.Title)
		printDryRunField(&output, "Branch", options.ReleaseBranch+" → "+options.BaseBranch)

		body, err := renderDryRunBody(options.Body)
		if err != nil {
			return "", fmt.Errorf("render pull request body preview: %w", err)
		}

		_, _ = fmt.Fprintln(&output)
		_, _ = fmt.Fprintln(&output, ui.Header.Render("Body Preview"))
		_, _ = fmt.Fprintln(&output)
		_, _ = fmt.Fprintln(&output, trimDryRunBody(body))
	}

	_, _ = fmt.Fprintln(&output)
	printDryRunSummary(&output, len(result.Plans), commitCount, pullRequestCount)

	return output.String(), nil
}

func printDryRunTarget(w io.Writer, plan release.TargetPlan) {
	printDryRunField(w, "Target", plan.ID)
	printDryRunField(w, "Version", plan.CurrentVersion+" → "+ui.Value.Render(plan.NextVersion))
	printDryRunField(w, "Bump", string(plan.BumpType))
	printDryRunField(w, "Tag", ui.Value.Render(plan.NextTag))
	printDryRunField(w, "Commits", strconv.Itoa(plan.CommitCount))
}

func printDryRunField(w io.Writer, label string, value string) {
	paddedLabel := fmt.Sprintf("%-10s", label)
	_, _ = fmt.Fprintf(w, "  %s  %s\n", ui.Faint.Render(paddedLabel), value)
}

func printDryRunSummary(w io.Writer, targets int, commits int, pullRequests int) {
	_, _ = fmt.Fprintf(
		w,
		"Plan: %d %s, %d %s, %d %s.\n",
		targets,
		pluralize(targets, "target"),
		commits,
		pluralize(commits, "commit"),
		pullRequests,
		pluralize(pullRequests, "pull request"),
	)
}

func pluralize(count int, singular string) string {
	if count == 1 {
		return singular
	}

	return singular + "s"
}

func renderDryRunBody(markdown string) (string, error) {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(dryRunBodyStyle()),
		glamour.WithWordWrap(dryRunPreviewWidth),
	)
	if err != nil {
		return "", fmt.Errorf("create Markdown renderer: %w", err)
	}

	body, err := renderer.Render(markdown)
	if err != nil {
		return "", fmt.Errorf("render Markdown: %w", err)
	}

	body = strings.ReplaceAll(body, terminalansi.ResetHyperlink()+" ", terminalansi.ResetHyperlink())

	return body, nil
}

func trimDryRunBody(body string) string {
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}

	return strings.Join(lines, "\n")
}

func dryRunBodyStyle() glamouransi.StyleConfig {
	style := styles.ASCIIStyleConfig

	style.Document.BlockPrefix = ""
	style.Document.BlockSuffix = ""
	style.H1.Prefix = ""
	style.H2.Prefix = ""
	style.H3.Prefix = ""
	style.H4.Prefix = ""
	style.H5.Prefix = ""
	style.H6.Prefix = ""
	style.Emph = glamouransi.StylePrimitive{}
	style.Strong = glamouransi.StylePrimitive{}
	style.Link = glamouransi.StylePrimitive{Format: `{{""}}`}
	style.LinkText = glamouransi.StylePrimitive{}

	return style
}
