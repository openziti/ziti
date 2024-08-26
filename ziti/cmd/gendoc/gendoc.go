package zssh

import (
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
	"os"
	"path"
	"strings"
)

func NewGendocCmd(rootCmd *cobra.Command) *cobra.Command {
	var docDir string
	var docCmd = &cobra.Command{
		Use:    "gendoc",
		Short:  "Generate Markdown documentation for the app",
		Hidden: true,
		Run: func(cmd *cobra.Command, args []string) {
			if _, err := os.Stat("docs"); os.IsExist(err) {
				_ = os.RemoveAll("docs")
			}

			err := os.MkdirAll("docs", os.ModePerm) // Create the directory with appropriate permissions
			if err != nil {
				logrus.Fatalf("Failed to create docs directory: %v", err)
			}
			if docDir == "" {
				docDir = "docs"
			}
			toMd(rootCmd, docDir)
		},
	}
	docCmd.Flags().StringVar(&docDir, "doc-output-dir",  "", "the directory to output the docs to")
	return docCmd
}

func toMd(child *cobra.Command, outputDir string) {
	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		err := os.MkdirAll(outputDir, os.ModePerm)
		if err != nil {
			logrus.Fatalf("Failed to create docs directory: %v", err)
		}
	}
	filePath := path.Join(outputDir, child.Name()+".md")
	file, err := os.Create(filePath)
	if err != nil {
		panic(err)
	}

	capturedFunc := func(s string) string {
		isChildCmd := strings.Contains(s, child.Name())
		//captured here to capture the current variables needed to output the path in the callback
		parts := strings.Split(s, "_")
		partsLen := len(parts)
		n := parts[partsLen-1]
		n = n[:strings.Index(n, ".")]
		if isChildCmd {
			return path.Join(n, n+".md")
		} else {
			return path.Join("..", n+".md")
		}
	}
	err = doc.GenMarkdownCustom(child, file, capturedFunc)
	if err != nil {
		panic(err)
	}

	_ = file.Close()

	for _, subCmd := range child.Commands() {
		if !subCmd.Hidden {
			toMd(subCmd, path.Join(outputDir, subCmd.Name()))
		}
	}
}
