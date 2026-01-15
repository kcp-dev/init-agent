/*
Copyright 2026 The kcp Authors.

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

package version

// These variables get fed by ldflags during compilation.
var (
	// GitVersion is a variable containing the git commit identifier
	// (usually the output of `git describe`, i.e. not necessarily a
	// static tag name); for a tagged Init Agent release, this value is identical
	// to kubermaticDockerTag, for untagged builds this is the `git describe`
	// output.
	GitVersion string
	// GitHead is the full SHA hash of the Git commit the application was built for.
	GitHead string
)
