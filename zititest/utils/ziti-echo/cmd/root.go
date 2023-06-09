package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var identityFile string

var rootCmd = &cobra.Command{
	Use:   "ziti-echo",
	Short: "A simple Echo Service",
}

func init() {
	rootCmd.PersistentFlags().StringVar(&identityFile, "identity", "i", "ziti identity file")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
