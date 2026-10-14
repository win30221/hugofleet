package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/win30221/hugofleet/stages/m5/observability"
	"github.com/win30221/hugofleet/stages/m5/traceview"
)

type paths []string

func (p *paths) String() string     { return fmt.Sprint([]string(*p)) }
func (p *paths) Set(v string) error { *p = append(*p, v); return nil }
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "trace viewer:", err)
		os.Exit(1)
	}
}
func run() error {
	f := flag.NewFlagSet("traceview", flag.ContinueOnError)
	var inputs paths
	f.Var(&inputs, "input", "collected JSON log, repeat at most eight times")
	output := f.String("output", "", "standalone safe HTML destination")
	if err := f.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if len(inputs) < 1 || len(inputs) > 8 || *output == "" || f.NArg() != 0 {
		return fmt.Errorf("1..8 collected inputs and explicit output required")
	}
	items := []observability.Record{}
	for _, path := range inputs {
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		remaining := 20000 - len(items)
		if remaining == 0 {
			file.Close()
			break
		}
		records, e := traceview.Read(file, remaining)
		closeErr := file.Close()
		if e != nil || closeErr != nil {
			return errors.Join(e, closeErr)
		}
		items = append(items, records...)
	}
	file, err := os.OpenFile(*output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	// html/template makes many small writes. Buffer them so collected traces
	// remain practical on a Windows-mounted checkout as well as native Linux.
	buffer := bufio.NewWriterSize(file, 64<<10)
	renderErr := traceview.Render(buffer, items)
	flushErr := buffer.Flush()
	return errors.Join(renderErr, flushErr, file.Close())
}
