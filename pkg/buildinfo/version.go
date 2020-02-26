/*
Copyright 2017 the Velero contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package buildinfo holds build-time information like the velero version.
// This is a separate package so that other packages can import it without
// worrying about introducing circular dependencies.
package buildinfo

import "fmt"

var (
	// Version is the current version of Velero plugins, set by the go linker's -X flag at build time.
	Version string

	// Registry is the current registry of image of Velero plugins, set by the go linker's -X flag at build time.
	Registry string

	// LocalMode is the flag to control whether to enable the data manager feature or not, set by the go linker's -X flag at build time.
	LocalMode string

	// GitSHA is the actual commit that is being built, set by the go linker's -X flag at build time.
	GitSHA string

	// GitTreeState indicates if the git tree is clean or dirty, set by the go linker's -X flag at build
	// time.
	GitTreeState string
)

// FormattedGitSHA renders the Git SHA with an indicator of the tree state.
func FormattedGitSHA() string {
	if GitTreeState != "clean" {
		return fmt.Sprintf("%s-%s", GitSHA, GitTreeState)
	}
	return GitSHA
}
