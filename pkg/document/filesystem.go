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
	"io/ioutil"

	fs "sigs.k8s.io/kustomize/api/filesys"
)

// File extends kustomize File and provide abstraction to creating temporary files
type File interface {
	fs.File
	Name() string
}

// FileSystem extends kustomize FileSystem and provide abstraction to creating temporary files
type FileSystem interface {
	fs.FileSystem
	TempFile(string, string) (File, error)
	TempDir(string, string) (string, error)
}

// Fs is adaptor to TempFile
type Fs struct {
	fs.FileSystem
}

// NewDocumentFs returns an instance of Fs
func NewDocumentFs() FileSystem {
	return &Fs{FileSystem: fs.MakeFsOnDisk()}
}

// TempFile creates file in temporary filesystem, at default os.TempDir
func (dfs Fs) TempFile(tmpDir string, prefix string) (File, error) {
	return ioutil.TempFile(tmpDir, prefix)
}

// TempDir creates a temporary directory in given root directory
func (dfs Fs) TempDir(rootDir string, prefix string) (string, error) {
	return ioutil.TempDir(rootDir, prefix)
}
