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

package isogen

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/cheggaaa/pb/v3"

	"opendev.org/airship/airshipctl/pkg/api/v1alpha1"
	"opendev.org/airship/airshipctl/pkg/bootstrap/cloudinit"
	"opendev.org/airship/airshipctl/pkg/config"
	"opendev.org/airship/airshipctl/pkg/container"
	"opendev.org/airship/airshipctl/pkg/document"
	"opendev.org/airship/airshipctl/pkg/log"
	"opendev.org/airship/airshipctl/pkg/util"
)

const (
	builderConfigFileName = "builder-conf.yaml"

	// progressBarTemplate is a template string for progress bar
	// looks like 'Prefix [-->______] 20%' where Prefix is trimmed log line from docker container
	progressBarTemplate = `{{string . "prefix"}} {{bar . }} {{percent . }} `
	// defaultTerminalWidth is a default width of terminal if it's impossible to determine the actual one
	defaultTerminalWidth = 80
	// multiplier is a number of log lines produces while installing 1 package
	multiplier = 3
	// reInstallActions is a regular expression to check whether the log line contains of this substrings
	reInstallActions = `Extracting|Unpacking|Configuring|Preparing|Setting`
	reInstallBegin   = `Retrieving Packages|newly installed`
	reInstallFinish  = `Base system installed successfully|mksquashfs`
)

// BootstrapIsoOptions are used to generate bootstrap ISO
type BootstrapIsoOptions struct {
	docBundle document.Bundle
	builder   container.Container
	doc       document.Document
	cfg       *v1alpha1.ImageConfiguration

	// optional fields for verbose output
	debug    bool
	progress bool
	writer   io.Writer
}

// GenerateBootstrapIso will generate data for cloud init and start ISO builder container
// TODO (vkuzmin): Remove this public function and move another functions
// to the executor module when the phases will be ready
func GenerateBootstrapIso(cfgFactory config.Factory, progress bool) error {
	ctx := context.Background()

	globalConf, err := cfgFactory()
	if err != nil {
		return err
	}

	root, err := globalConf.CurrentContextEntryPoint(config.BootstrapPhase)
	if err != nil {
		return err
	}
	docBundle, err := document.NewBundleByPath(root)
	if err != nil {
		return err
	}

	imageConfiguration := &v1alpha1.ImageConfiguration{}
	selector, err := document.NewSelector().ByObject(imageConfiguration, v1alpha1.Scheme)
	if err != nil {
		return err
	}
	doc, err := docBundle.SelectOne(selector)
	if err != nil {
		return err
	}

	err = doc.ToAPIObject(imageConfiguration, v1alpha1.Scheme)
	if err != nil {
		return err
	}
	if err = verifyInputs(imageConfiguration); err != nil {
		return err
	}

	log.Print("Creating ISO builder container")
	builder, err := container.NewContainer(
		&ctx, imageConfiguration.Container.ContainerRuntime,
		imageConfiguration.Container.Image)
	if err != nil {
		return err
	}

	bootstrapIsoOptions := BootstrapIsoOptions{
		docBundle: docBundle,
		builder:   builder,
		doc:       doc,
		cfg:       imageConfiguration,
		debug:     log.DebugEnabled(),
		progress:  progress,
		writer:    log.Writer(),
	}
	err = bootstrapIsoOptions.createBootstrapIso()
	if err != nil {
		return err
	}
	log.Print("Checking artifacts")
	return verifyArtifacts(imageConfiguration)
}

func verifyInputs(cfg *v1alpha1.ImageConfiguration) error {
	if cfg.Container.Volume == "" {
		return config.ErrMissingConfig{
			What: "Must specify volume bind for ISO builder container",
		}
	}

	if (cfg.Builder.UserDataFileName == "") || (cfg.Builder.NetworkConfigFileName == "") {
		return config.ErrMissingConfig{
			What: "UserDataFileName or NetworkConfigFileName are not specified in ISO builder config",
		}
	}

	vols := strings.Split(cfg.Container.Volume, ":")
	switch {
	case len(vols) == 1:
		cfg.Container.Volume = fmt.Sprintf("%s:%s", vols[0], vols[0])
	case len(vols) > 2:
		return config.ErrInvalidConfig{
			What: "Bad container volume format. Use hostPath:contPath",
		}
	}
	return nil
}

func getContainerCfg(
	cfg *v1alpha1.ImageConfiguration,
	builderCfgYaml []byte,
	userData []byte,
	netConf []byte,
) map[string][]byte {
	hostVol := strings.Split(cfg.Container.Volume, ":")[0]

	fls := make(map[string][]byte)
	fls[filepath.Join(hostVol, cfg.Builder.UserDataFileName)] = userData
	fls[filepath.Join(hostVol, cfg.Builder.NetworkConfigFileName)] = netConf
	fls[filepath.Join(hostVol, builderConfigFileName)] = builderCfgYaml
	return fls
}

func verifyArtifacts(cfg *v1alpha1.ImageConfiguration) error {
	hostVol := strings.Split(cfg.Container.Volume, ":")[0]
	metadataPath := filepath.Join(hostVol, cfg.Builder.OutputMetadataFileName)
	_, err := os.Stat(metadataPath)
	return err
}

func (opts BootstrapIsoOptions) createBootstrapIso() error {
	cntVol := strings.Split(opts.cfg.Container.Volume, ":")[1]
	log.Print("Creating cloud-init for ephemeral K8s")
	userData, netConf, err := cloudinit.GetCloudData(opts.docBundle)
	if err != nil {
		return err
	}

	builderCfgYaml, err := opts.doc.AsYAML()
	if err != nil {
		return err
	}

	fls := getContainerCfg(opts.cfg, builderCfgYaml, userData, netConf)
	if err = util.WriteFiles(fls, 0600); err != nil {
		return err
	}

	vols := []string{opts.cfg.Container.Volume}
	builderCfgLocation := filepath.Join(cntVol, builderConfigFileName)
	log.Printf("Running default container command. Mounted dir: %s", vols)

	envVars := []string{
		fmt.Sprintf("BUILDER_CONFIG=%s", builderCfgLocation),
		fmt.Sprintf("http_proxy=%s", os.Getenv("http_proxy")),
		fmt.Sprintf("https_proxy=%s", os.Getenv("https_proxy")),
		fmt.Sprintf("HTTP_PROXY=%s", os.Getenv("HTTP_PROXY")),
		fmt.Sprintf("HTTPS_PROXY=%s", os.Getenv("HTTPS_PROXY")),
		fmt.Sprintf("NO_PROXY=%s", os.Getenv("NO_PROXY")),
	}

	err = opts.builder.RunCommand([]string{}, nil, vols, envVars)
	if err != nil {
		return err
	}

	log.Print("ISO generation is in progress. The whole process could take up to several minutes, please wait...")

	if opts.debug || opts.progress {
		var cLogs io.ReadCloser
		cLogs, err = opts.builder.GetContainerLogs()
		if err != nil {
			log.Printf("failed to read container logs %s", err)
		} else {
			switch {
			case opts.progress:
				if err = showProgress(cLogs, opts.writer); err != nil {
					log.Debugf("the following error occurred while showing progress bar: %s", err.Error())
				}
			case opts.debug:
				log.Print("start reading container logs")
				// either container log output or progress bar will be shown
				if _, err = io.Copy(log.Writer(), cLogs); err != nil {
					log.Debugf("failed to write container logs to log output %s", err)
				}
				log.Print("got EOF from container logs")
			}
		}
	}

	if err = opts.builder.WaitUntilFinished(); err != nil {
		return err
	}

	log.Print("ISO successfully built.")
	if !opts.debug {
		log.Print("Removing container.")
		return opts.builder.RmContainer()
	}

	log.Debugf("Debug flag is set. Container %s stopped but not deleted.", opts.builder.GetID())
	return nil
}

func showProgress(reader io.ReadCloser, writer io.Writer) error {
	reFindActions := regexp.MustCompile(reInstallActions)
	reBeginInstall := regexp.MustCompile(reInstallBegin)
	reFinishInstall := regexp.MustCompile(reInstallFinish)

	var bar *pb.ProgressBar

	scanner := bufio.NewScanner(reader)
	scanner.Split(bufio.ScanLines)
	// Reading container log line by line
	for scanner.Scan() {
		curLine := scanner.Text()
		// Trying to find entry points of package installation
		switch {
		case reBeginInstall.MatchString(curLine):
			if err := finalizePb(bar, nil); err != nil {
				return err
			}

			pkgCount, err := calculatePkgCount(scanner, writer, curLine)
			if err != nil {
				return finalizePb(bar, err)
			}

			bar, err = initPb(pkgCount, writer)
			if err != nil {
				return err
			}
		case reFinishInstall.MatchString(curLine):
			if err := finalizePb(bar, nil); err != nil {
				return err
			}
		case reFindActions.MatchString(curLine):
			if err := incrementPb(bar, curLine); err != nil {
				return finalizePb(bar, err)
			}
		case strings.Contains(curLine, "filesystem.squashfs"):
			fmt.Fprintln(writer, curLine)
		}
	}

	if bar != nil && bar.IsStarted() {
		return finalizePb(bar, ErrUnexpectedPb{})
	}

	return nil
}

func finalizePb(bar *pb.ProgressBar, e error) error {
	if bar != nil && bar.IsStarted() {
		bar.SetCurrent(bar.Total())
		if e != nil {
			setPbPrefix(bar, "An error occurred while log parsing")
			bar.Finish()
			return e
		}

		setPbPrefix(bar, "Completed")
		bar.Finish()
		if err := bar.Err(); err != nil {
			return err
		}
	}
	return e
}

func initPb(pkgCount int, w io.Writer) (*pb.ProgressBar, error) {
	bar := pb.ProgressBarTemplate(progressBarTemplate).New(pkgCount * multiplier)
	bar.SetWriter(w).Start()
	setPbPrefix(bar, "Installing required packages")
	if err := bar.Err(); err != nil {
		return nil, finalizePb(bar, err)
	}
	return bar, nil
}

func incrementPb(bar *pb.ProgressBar, curLine string) error {
	if bar != nil && bar.IsStarted() && bar.Current() < bar.Total() {
		setPbPrefix(bar, curLine)
		bar.Increment()
		if err := bar.Err(); err != nil {
			return finalizePb(bar, err)
		}
	}
	return nil
}

func setPbPrefix(bar *pb.ProgressBar, msg string) {
	terminalWidth := defaultTerminalWidth
	halfWidth := terminalWidth / 2
	bar.SetWidth(terminalWidth)
	if len(msg) > halfWidth {
		msg = fmt.Sprintf("%v...", msg[0:halfWidth-3])
	} else {
		msg = fmt.Sprintf("%-*v", halfWidth, msg)
	}
	bar.Set("prefix", msg)
}

func calculatePkgCount(scanner *bufio.Scanner, writer io.Writer, curLine string) (int, error) {
	reFindNumbers := regexp.MustCompile("[0-9]+")

	// Trying to count how many packages is going to be installed
	pkgCount := 0
	matches := reFindNumbers.FindAllString(curLine, -1)
	if matches == nil {
		// There is no numbers in line about base packages, counting them manually to get estimates
		fmt.Fprint(writer, "Retrieving base packages ")
		for scanner.Scan() {
			curLine = scanner.Text()
			if strings.Contains(curLine, "Retrieving") {
				pkgCount++
				fmt.Fprint(writer, ".")
			}
			if strings.Contains(curLine, "Chosen extractor") {
				fmt.Fprintln(writer, " Done")
				return pkgCount, nil
			}
		}
	}
	if len(matches) >= 2 {
		for _, v := range matches[0:2] {
			j, err := strconv.Atoi(v)
			if err != nil {
				continue
			}
			pkgCount += j
		}
		if pkgCount > 0 {
			return pkgCount, nil
		}
	}

	return pkgCount, ErrNoParsedNumPkgs{}
}
