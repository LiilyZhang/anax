package helm

import (
	"errors"
	"fmt"
	"github.com/golang/glog"
	"os/exec"
	"strings"
)

// This client implements our abstract helm client interface, using the Helm CLI.

type CliClient struct {
}

const INSTALL_ARGS = "install -n %v %v --kubeconfig /root/.kube/config"
const UNINSTALL_ARGS = "delete --purge %v --kubeconfig /root/.kube/config"
const STATUS_ARGS = "list -a --kubeconfig /root/.kube/config"
const DEPLOYED = "DEPLOYED"

const EOL = "\x0a"
const TAB = "\x09"

func NewCliClient() *CliClient {
	return new(CliClient)
}

func (c *CliClient) Install(b64Package string, releaseName string) error {

	if fileName, err := ConvertB64StringToFile(b64Package); err != nil {
		return errors.New(fmt.Sprintf("error converting Helm package to file: %v", err))
	} else {
		glog.V(5).Infof(clilogString(fmt.Sprintf("Decoded Helm package to file: %v", fileName)))
		args := fmt.Sprintf(INSTALL_ARGS, releaseName, fileName)
		glog.V(3).Infof(clilogString(fmt.Sprintf("Installing Helm package: %v", args)))
		argFields := strings.Fields(args)
		/*
		kubeConfigFile := "/root/.kube/config"
		args2 := fmt.Sprintf("--kubeconfig %v", kubeConfigFile)
		glog.V(3).Infof(clilogString(fmt.Sprintf("Installing Helm package args2: %v", args2)))
		glog.V(3).Infof(clilogString(fmt.Sprintf("Set helm kubeconfig to: %v", kubeConfigFile)))

		helmCmd := fmt.Sprintf("helm %v %v", args, args2)
		glog.V(3).Infof(clilogString(fmt.Sprintf("helmCmd: %v", helmCmd)))*/

		/*
		if out, err := exec.Command("helm", "--kubeconfig", kubeConfigFile).Output(); err != nil {
			errMsg := ""
			if exErr, ok := err.(*exec.ExitError); ok {
				errMsg = string(exErr.Stderr)
				glog.V(3).Infof(clilogString(fmt.Sprintf("Error setting helm kubeconfig to: %v, error message: %v", kubeConfigFile, errMsg)))
			}
		} else {
			glog.V(3).Infof(clilogString(fmt.Sprintf("Output from set helm configl: (%T) %s", out, string(out))))
		}*/

		if out, err := exec.Command("helm", argFields...).Output(); err != nil {
			errMsg := ""
			if exErr, ok := err.(*exec.ExitError); ok {
				errMsg = string(exErr.Stderr)
			}
			return errors.New(fmt.Sprintf("error installing Helm package: (%T) %v error message: %v", err, err, errMsg))
		} else {
			glog.V(5).Infof(clilogString(fmt.Sprintf("Output from install: (%T) %s", out, string(out))))
		}
	}

	return nil
}

func (c *CliClient) UnInstall(releaseName string) error {

	args := fmt.Sprintf(UNINSTALL_ARGS, releaseName)
	glog.V(5).Infof(clilogString(fmt.Sprintf("Uninstalling Helm package: %v", args)))
	argFields := strings.Fields(args)
	if out, err := exec.Command("helm", argFields...).Output(); err != nil {
		errMsg := ""
		if exErr, ok := err.(*exec.ExitError); ok {
			errMsg = string(exErr.Stderr)
		}
		return errors.New(fmt.Sprintf("error uninstalling Helm package: (%T) %v error message: %v", err, err, errMsg))
	} else {
		glog.V(5).Infof(clilogString(fmt.Sprintf("Output from uninstall: (%T) %s", out, string(out))))
	}

	return nil
}

func (c *CliClient) Status(releaseName string) (*ReleaseStatus, error) {

	status := new(ReleaseStatus)

	args := fmt.Sprintf(STATUS_ARGS)
	glog.V(3).Infof(clilogString(fmt.Sprintf("Listing Helm releases: %v, args %v", releaseName, args)))
	argFields := strings.Fields(args)
	if out, err := exec.Command("helm", argFields...).Output(); err != nil {
		errMsg := ""
		if exErr, ok := err.(*exec.ExitError); ok {
			errMsg = string(exErr.Stderr)
		}
		return nil, errors.New(fmt.Sprintf("error listing Helm releases: (%T) %v error message: %v", err, err, errMsg))
	} else {

		glog.V(3).Infof(clilogString(fmt.Sprintf("Output from list releases: (%T) %s", out, string(out))))

		// Split std out into lines (array of string). There should be at least 2 lines if there is anything deployed.
		lines := strings.Split(string(out), EOL)
		if len(lines) <= 1 {
			return nil, errors.New(fmt.Sprintf("no active releases"))
		}
		glog.V(3).Infof(clilogString(fmt.Sprintf("Output as lines: %v", lines)))

		// Find the line that starts with our release.
		for _, line := range lines {
			tabs := strings.Split(line, TAB)
			glog.V(3).Infof(clilogString(fmt.Sprintf("Output as tabs: %v", tabs)))
			if len(tabs) < 7 {
				//return nil, errors.New(fmt.Sprintf("not enough output, expecting 6 tabs: %v", line))
				glog.V(3).Infof(clilogString(fmt.Sprintf("not enough output, expecting 6 tabs: %v", line)))
				continue
			} else {
				glog.V(3).Infof(clilogString(fmt.Sprintf("tabs[0]: %v, tabs[1]: %v, tabs[2]: %v, tabs[3]: %v, tabs[4]: %v, tabs[5]: %v, tabs[6]: %v", tabs[0],
					tabs[1], tabs[2], tabs[3], tabs[4], tabs[5], tabs[6])))
				if strings.TrimSpace(tabs[0]) == releaseName {
					glog.V(3).Infof(clilogString(fmt.Sprintf("Found release %v", releaseName)))
					status.Name = strings.TrimSpace(tabs[0])
					status.Revision = strings.TrimSpace(tabs[1])
					status.Updated = strings.TrimSpace(tabs[2])
					status.Status = strings.TrimSpace(tabs[3])
					status.ChartName = strings.TrimSpace(tabs[4])
					status.AppVersion = strings.TrimSpace(tabs[5])
					status.Namespace = strings.TrimSpace(tabs[6])
					glog.V(3).Infof(clilogString(fmt.Sprintf("Found release info %v", status)))
					return status, nil
				}
			}
		}
		return nil, errors.New(fmt.Sprintf("release not found: %v", lines))
	}

}

// Helm time format. Golang requires the format string to be in reference to the specific time as shown.
// This is so that the formatter and parser can figure out what goes where in the string.
const HelmCLIReleaseStatusTimeFormat = "Mon Jan 2 15:04:05 2006"

func (c *CliClient) ReleaseTimeFormat() string {
	return HelmCLIReleaseStatusTimeFormat
}

var clilogString = func(v interface{}) string {
	return fmt.Sprintf("Helm CliClient: %v", v)
}
