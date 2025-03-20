package persistence

import (
	"encoding/json"
	"fmt"
	"github.com/open-horizon/anax/cutil"
)

// The structure of the json string in the deployment field of a service definition when the
// service is deployed via Helm to a Kubernetes cluster.

type Helm3DeploymentConfig struct {
	ChartArchive string                 `json:"chartArchive"` // base64 encoded binary of helm3 package tar file
	ReleaseName  string                 `json:"releaseName"`
	Secrets      map[string]interface{} `json:"secrets,omitempty"`
}

func (h *Helm3DeploymentConfig) ToString() string {
	if h != nil {
		return fmt.Sprintf("ChartArchive: %v, Release Name: %v, Secrets: %v", cutil.TruncateDisplayString(h.ChartArchive, 20), h.ReleaseName, h.Secrets)
	}
	return ""
}

// Given a deployment string, unmarshal it as a Helm3Deployment object. It might not be a Helm3Deployment, so
// we have to verify what was just unmarshalled.
func GetHelm3Deployment(depStr string) (*Helm3DeploymentConfig, error) {
	hd := new(Helm3DeploymentConfig)
	err := json.Unmarshal([]byte(depStr), hd)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling deployment config as Helm3Deployment: %v", err)
	} else if hd.ChartArchive == "" {
		return nil, fmt.Errorf("required field 'chartArchive' is missing in the deployment string")
	}

	return hd, nil
}

func IsHelm3(dep map[string]interface{}) bool {
	if _, ok := dep["chartArchive"]; ok {
		return true
	}
	return false
}

// Functions that allow HelmDeploymentConfig to support the DeploymentConfig interface.

func (h *Helm3DeploymentConfig) IsNative() bool {
	return false
}

func (h *Helm3DeploymentConfig) IsKube() bool {
	return false
}

func (h *Helm3DeploymentConfig) FromPersistentForm(pf map[string]interface{}) error {
	// Marshal to JSON form so that we can unmarshal as a Helm3DeploymentConfig.
	if jBytes, err := json.Marshal(pf); err != nil {
		return fmt.Errorf("error marshalling helm3 persistent deployment: %v, error: %v", h, err)
	} else if err := json.Unmarshal(jBytes, h); err != nil {
		return fmt.Errorf("error unmarshalling helm3 persistent deployment: %v, error: %v", string(jBytes), err)
	}
	return nil
}

func (h *Helm3DeploymentConfig) ToPersistentForm() (map[string]interface{}, error) {
	pf := make(map[string]interface{})

	// Marshal to JSON form so that we can unmarshal as a map[string]interface{}.
	if jBytes, err := json.Marshal(h); err != nil {
		return pf, fmt.Errorf("error marshalling helm3 deployment: %v, error: %v", h, err)
	} else if err := json.Unmarshal(jBytes, &pf); err != nil {
		return pf, fmt.Errorf("error unmarshalling helm3 deployment: %v, error: %v", string(jBytes), err)
	}

	return pf, nil
}
