package commands

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/monkescience/yeet/internal/build"
)

type buildInfo struct {
	name      string
	version   string
	commit    string
	built     string
	platform  string
	goVersion string
	moduleSum string
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Print build information",
		Example: `  yeet version`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return printVersion(cmd.OutOrStdout(), currentBuildInfo())
		},
	}
}

func currentBuildInfo() buildInfo {
	return buildInfo{
		name:      build.ServiceName,
		version:   build.Version(),
		commit:    build.Commit(),
		built:     build.Date(),
		platform:  build.Platform(),
		goVersion: build.GoVersion(),
		moduleSum: build.Module(),
	}
}

func printVersion(w io.Writer, info buildInfo) error {
	_, err := fmt.Fprintf(w, "name: %s\nversion: %s\n", info.name, info.version)
	if err != nil {
		return fmt.Errorf("print version: %w", err)
	}

	if info.moduleSum != "" {
		_, err = fmt.Fprintf(w, "module-sum: %s\n", info.moduleSum)
	} else {
		_, err = fmt.Fprintf(w, "commit: %s\n", info.commit)
	}

	if err != nil {
		return fmt.Errorf("print commit: %w", err)
	}

	_, err = fmt.Fprintf(
		w,
		"built: %s\nplatform: %s\ngo-version: %s\n",
		info.built, info.platform, info.goVersion,
	)
	if err != nil {
		return fmt.Errorf("print build info: %w", err)
	}

	return nil
}
