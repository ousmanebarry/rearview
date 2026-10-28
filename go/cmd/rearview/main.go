package main

import (
	"fmt"
	"io"
	"os"
	_ "time/tzdata" // Windows has no zoneinfo database for Go to load.

	"github.com/spf13/cobra"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "0.0.0-dev"

func main() {
	if err := newRootCmd(os.Stdout, os.Stderr).Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd(out, errOut io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:          "rearview",
		Short:        "Dashcam footage analyzer",
		Version:      version,
		SilenceUsage: true,
	}
	root.SetOut(out)
	root.SetErr(errOut)
	root.SetVersionTemplate("rearview {{.Version}}\n")
	root.AddCommand(
		&cobra.Command{
			Use:   "version",
			Short: "Print the rearview version",
			Args:  cobra.NoArgs,
			Run: func(cmd *cobra.Command, _ []string) {
				fmt.Fprintf(cmd.OutOrStdout(), "rearview %s\n", version)
			},
		},
		newLabelCmd(),
	)
	return root
}
