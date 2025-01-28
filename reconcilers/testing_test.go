/*
Copyright 2020 the original author or authors.

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

package reconcilers_test

import (
	"github.com/go-logr/logr"
)

var _ logr.LogSink = &bufferedSink{}

type bufferedSink struct {
	Lines []string
}

func (s *bufferedSink) Init(info logr.RuntimeInfo) {}
func (s *bufferedSink) Enabled(level int) bool {
	return true
}
func (s *bufferedSink) Info(level int, msg string, keysAndValues ...interface{}) {
	s.Lines = append(s.Lines, msg)
}
func (s *bufferedSink) Error(err error, msg string, keysAndValues ...interface{}) {
	s.Lines = append(s.Lines, msg)
}
func (s *bufferedSink) WithValues(keysAndValues ...interface{}) logr.LogSink {
	return s
}
func (s *bufferedSink) WithName(name string) logr.LogSink {
	return s
}
