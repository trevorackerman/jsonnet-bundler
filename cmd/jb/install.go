// Copyright 2018 jsonnet-bundler authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"reflect"

	"github.com/pkg/errors"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/trevorackerman/jsonnet-bundler/pkg"
	"github.com/trevorackerman/jsonnet-bundler/pkg/jsonnetfile"
	v1 "github.com/trevorackerman/jsonnet-bundler/spec/v1"
	"github.com/trevorackerman/jsonnet-bundler/spec/v1/deps"
)

func installCommand(dir, jsonnetHome string, uris []string, single bool, legacyName string) int {
	if dir == "" {
		dir = "."
	}

	jbfilebytes, err := ioutil.ReadFile(filepath.Join(dir, jsonnetfile.File))
	kingpin.FatalIfError(err, "failed to load jsonnetfile")

	jsonnetFile, err := jsonnetfile.Unmarshal(jbfilebytes)
	kingpin.FatalIfError(err, "")

	jblockfilebytes, err := ioutil.ReadFile(filepath.Join(dir, jsonnetfile.LockFile))
	if !os.IsNotExist(err) {
		kingpin.FatalIfError(err, "failed to load lockfile")
	}

	lockFile, err := jsonnetfile.Unmarshal(jblockfilebytes)
	kingpin.FatalIfError(err, "")

	kingpin.FatalIfError(
		os.MkdirAll(filepath.Join(dir, jsonnetHome, ".tmp"), os.ModePerm),
		"creating vendor folder")

	if len(uris) > 1 && legacyName != "" {
		log.Fatal("Cannot use --legacy-name with mutliple uris")
	}

	for _, u := range uris {
		d := deps.Parse(dir, u)
		if d == nil {
			kingpin.Fatalf("Unable to parse package URI `%s`", u)
		}

		if single {
			d.Single = true
		}

		if legacyName != "" {
			d.LegacyNameCompat = legacyName
		}

		jd, _ := jsonnetFile.Dependencies.Get(d.Name())
		if !depEqual(jd, *d) {
			// the dep passed on the cli is different from the jsonnetFile
			jsonnetFile.Dependencies.Set(d.Name(), *d)

			// we want to install the passed version (ignore the lock)
			lockFile.Dependencies.Delete(d.Name())
		}
	}

	jsonnetPkgHomeDir := filepath.Join(dir, jsonnetHome)
	fmt.Println("Installing packages into", jsonnetPkgHomeDir)
	locked, err := pkg.Ensure(jsonnetFile, jsonnetPkgHomeDir, lockFile.Dependencies)
	kingpin.FatalIfError(err, "failed to install packages")

	pkg.CleanLegacyName(jsonnetFile.Dependencies)

	kingpin.FatalIfError(
		writeChangedJsonnetFile(jbfilebytes, &jsonnetFile, filepath.Join(dir, jsonnetfile.File)),
		"updating jsonnetfile.json")

	kingpin.FatalIfError(
		writeChangedJsonnetFile(jblockfilebytes, &v1.JsonnetFile{Dependencies: locked}, filepath.Join(dir, jsonnetfile.LockFile)),
		"updating jsonnetfile.lock.json")

	return 0
}

func depEqual(d1, d2 deps.Dependency) bool {
	name := d1.Name() == d2.Name()
	version := d1.Version == d2.Version
	source := reflect.DeepEqual(d1.Source, d2.Source)

	return name && version && source
}

func writeJSONFile(name string, d interface{}) error {
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return errors.Wrap(err, "encoding json")
	}
	b = append(b, []byte("\n")...)

	return ioutil.WriteFile(name, b, 0644)
}

func writeChangedJsonnetFile(originalBytes []byte, modified *v1.JsonnetFile, path string) error {
	origJsonnetFile, err := jsonnetfile.Unmarshal(originalBytes)
	if err != nil {
		return err
	}

	if reflect.DeepEqual(origJsonnetFile, *modified) {
		return nil
	}

	return writeJSONFile(path, *modified)
}
