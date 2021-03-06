/*
 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     https://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package document

import (
	"fmt"
	"testing"

	"opendev.org/airship/airshipctl/testutil"
)

func TestPlugin(t *testing.T) {
	cmdTests := []*testutil.CmdTest{
		{
			Name:    "document-plugin-cmd-with-empty-args",
			CmdLine: "",
			Error:   fmt.Errorf("requires at least 1 arg(s), only received 0"),
			Cmd:     NewPluginCommand(),
		},
		{
			Name:    "document-plugin-cmd-with-nonexistent-config",
			CmdLine: "/some/random/path.yaml",
			Error:   fmt.Errorf("open /some/random/path.yaml: no such file or directory"),
			Cmd:     NewPluginCommand(),
		},
	}

	for _, tt := range cmdTests {
		testutil.RunTest(t, tt)
	}
}
