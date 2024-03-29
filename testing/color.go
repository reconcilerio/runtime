/*
Copyright 2023 the original author or authors.

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

package testing

import (
	"os"
	"strings"

	"github.com/fatih/color"
)

func init() {
	if _, ok := os.LookupEnv("COLOR_DIFF"); ok {
		DiffAddedColor.EnableColor()
		DiffRemovedColor.EnableColor()
	}
}

var (
	DiffAddedColor   = color.New(color.FgGreen)
	DiffRemovedColor = color.New(color.FgRed)
)

func ColorizeDiff(diff string) string {
	var b strings.Builder
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+"):
			b.WriteString(DiffAddedColor.Sprint(line))
		case strings.HasPrefix(line, "-"):
			b.WriteString(DiffRemovedColor.Sprint(line))
		default:
			b.WriteString(line)
		}
		b.WriteString("\n")
	}
	return b.String()
}
