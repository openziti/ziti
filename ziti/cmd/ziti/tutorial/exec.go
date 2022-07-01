package tutorial

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/openziti/foundation/v2/term"
	"io"
	"os"
	"os/exec"
	"strings"
)

func Exec(program string, colorStdout bool, args ...string) error {
	p := exec.Command(program, args...)

	stdErrWrapper := &WriterWrapper{
		c: color.New(color.FgRed),
		w: os.Stdout,
	}

	p.Stdin = os.Stdin

	if colorStdout {
		stdOutWrapper := &WriterWrapper{
			c: color.New(color.FgBlue),
			w: os.Stdout,
		}
		p.Stdout = stdOutWrapper
	} else {
		p.Stdout = os.Stdout
	}

	p.Stderr = stdErrWrapper
	if err := p.Run(); err != nil {
		return err
	}
	return nil
}

func Continue(prompt string, defaultValue bool) {
	resp, err := AskYesNoWithDefault(prompt, defaultValue)
	if err != nil {
		fmt.Printf("\nerror: %v. exiting\n", err)
		os.Exit(1)
	}

	if !resp {
		fmt.Print("\nok, exiting\n")
		os.Exit(0)
	}
}

func AskYesNo(prompt string) (bool, error) {
	filter := &yesNoFilter{}
	if _, err := Ask(prompt, filter.Accept); err != nil {
		return false, err
	}
	return filter.result, nil
}

func AskYesNoWithDefault(prompt string, defaultVal bool) (bool, error) {
	filter := &yesNoDefaultFilter{
		defaultVal: defaultVal,
	}
	if _, err := Ask(prompt, filter.Accept); err != nil {
		return false, err
	}
	return filter.result, nil
}

func Ask(prompt string, f func(string) bool) (string, error) {
	for {
		val, err := term.Prompt(prompt)
		if err != nil {
			return "", err
		}
		val = strings.TrimSpace(val)
		if f(val) {
			return val, nil
		}
		fmt.Printf("Invalid input: %v\n", val)
	}
}

type yesNoFilter struct {
	result bool
}

func (self *yesNoFilter) Accept(s string) bool {
	if strings.EqualFold("y", s) || strings.EqualFold("yes", s) {
		self.result = true
		return true
	}

	if strings.EqualFold("n", s) || strings.EqualFold("no", s) {
		self.result = false
		return true
	}

	return false
}

type yesNoDefaultFilter struct {
	result     bool
	defaultVal bool
}

func (self *yesNoDefaultFilter) Accept(s string) bool {
	if s == "" {
		self.result = self.defaultVal
		return true
	}

	if strings.EqualFold("y", s) || strings.EqualFold("yes", s) {
		self.result = true
		return true
	}

	if strings.EqualFold("n", s) || strings.EqualFold("no", s) {
		self.result = false
		return true
	}

	return false
}

type WriterWrapper struct {
	c *color.Color
	w io.Writer
}

func (self *WriterWrapper) Write(p []byte) (n int, err error) {
	return self.c.Fprint(self.w, string(p))
}
