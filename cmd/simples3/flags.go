package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
)

var flagErrHelp = errors.New("help requested")

type stringListFlag []string

func (f *stringListFlag) String() string {
	return fmt.Sprintf("%v", []string(*f))
}

func (f *stringListFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func newFlagSet(name string, usage func()) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = usage
	return fs
}

func parseFlagSet(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fs.Usage()
			return flagErrHelp
		}
		return usageErrorf("%v", err)
	}
	return nil
}
