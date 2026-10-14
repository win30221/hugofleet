// proto-tool extracts the checksum-verified official compiler into project tools.
package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) != 4 {
		return fmt.Errorf("usage: proto-tool ZIP SHA256 DEST")
	}
	b, err := os.ReadFile(os.Args[1])
	if err != nil {
		return err
	}
	h := sha256.Sum256(b)
	if hex.EncodeToString(h[:]) != os.Args[2] {
		return fmt.Errorf("compiler checksum mismatch")
	}
	z, err := zip.OpenReader(os.Args[1])
	if err != nil {
		return err
	}
	defer z.Close()
	for _, f := range z.File {
		if f.Name != "bin/protoc" {
			continue
		}
		dest := filepath.Join(os.Args[3], "protoc")
		if err = os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}
		r, e := f.Open()
		if e != nil {
			return e
		}
		defer r.Close()
		w, e := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
		if e != nil {
			return e
		}
		_, e = io.Copy(w, r)
		return errors.Join(e, w.Close())
	}
	return fmt.Errorf("compiler archive has no bin/protoc")
}
