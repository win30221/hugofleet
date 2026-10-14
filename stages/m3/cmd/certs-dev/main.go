package main

import (
	"flag"
	"fmt"
	"github.com/win30221/hugofleet/stages/m3/devcert"
	"os"
	"strings"
)

func main() {
	dir := flag.String("output", ".local/m3/certs", "private demo credential directory")
	ids := flag.String("clusters", "hugofleet-a,hugofleet-b", "registered cluster IDs")
	flag.Parse()
	if e := devcert.Generate(*dir, strings.Split(*ids, ",")); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
