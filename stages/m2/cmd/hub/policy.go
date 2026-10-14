package main

import (
	"encoding/json"
	"fmt"
	"github.com/win30221/hugofleet/stages/m2/hub/domain"
	"io"
	"os"
)

func loadPolicy(path string) (domain.Policy, error) {
	var p domain.Policy
	f, err := os.Open(path)
	if err != nil {
		return p, err
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 16<<10))
	d.DisallowUnknownFields()
	if err = d.Decode(&p); err != nil {
		return p, err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return p, fmt.Errorf("policy must contain a single JSON object")
	}
	return p, p.Validate()
}
