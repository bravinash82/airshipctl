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

package utils

import (
	"os"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

// FactoryFromKubeConfigPath returns a factory with the
// default Kubernetes resources for the given kube config path
func FactoryFromKubeConfigPath(kp string) cmdutil.Factory {
	kf := genericclioptions.NewConfigFlags(false)
	kf.KubeConfig = &kp
	return cmdutil.NewFactory(kf)
}

// Streams returns default IO streams object, like stdout, stdin, stderr
func Streams() genericclioptions.IOStreams {
	return genericclioptions.IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}
}
