/*
Copyright 2022.

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

package helpers

import (
	"strings"

	"github.com/docker/docker/pkg/namesgenerator"
)

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

func GenerateName(currentNames []string) string {
	for i := 0; i < 3; i++ {
		name := strings.Replace(namesgenerator.GetRandomName(0), "_", "-", 1)
		if !contains(currentNames, name) {
			return name
		}
	}
	for i := 0; i < 100; i++ {
		name := strings.Replace(namesgenerator.GetRandomName(i), "_", "-", 1)
		if !contains(currentNames, name) {
			return name
		}
	}
	panic("Could not generate name!")
}
