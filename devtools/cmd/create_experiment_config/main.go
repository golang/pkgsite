// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command create_experiment_config creates an experiment.yaml file, which will
// set a rollout of 100 for all experiments.
package main

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/ghodss/yaml"
	"golang.org/x/pkgsite/internal"
)

type Experiment struct {
	Name    string `yaml:"name"`
	Rollout int    `yaml:"rollout"`
}

func main() {
	data, err := experimentsYAML()
	if err != nil {
		log.Fatal(err)
	}
	if err := writeConfigFile(data); err != nil {
		log.Fatal(err)
	}
}

func experimentsYAML() ([]byte, error) {
	var exps []*Experiment
	for e := range internal.Experiments {
		exps = append(exps, &Experiment{
			Name:    e,
			Rollout: 100,
		})
	}
	sort.Slice(exps, func(i, j int) bool { return exps[i].Name < exps[j].Name })
	data := map[string][]*Experiment{"experiments": exps}
	return yaml.Marshal(&data)
}

func writeConfigFile(data []byte) error {
	path, err := os.Getwd()
	if err != nil {
		return err
	}
	p := strings.TrimSuffix(path, "/devtools/cmd/run_beta") + "/experiment.yaml"
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		// Ignore f.Close() error, since f.Write returned an error.
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Printf("Set `export GO_DISCOVERY_CONFIG_DYNAMIC=%q` to enable experiments.\n", p)
	return nil
}
