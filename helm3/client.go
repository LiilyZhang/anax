package helm3

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/golang/glog"
	"github.com/open-horizon/anax/cutil"
	"github.com/open-horizon/anax/persistence"
	olmv1scheme "github.com/operator-framework/api/pkg/operators/v1"
	olmv1alpha1scheme "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"helm.sh/helm/v3/pkg/action"
	h3chart "helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/release"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	v1scheme "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	v1beta1scheme "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/scale/scheme"
	"log"
	"os"
	"regexp"
	"strings"
)

//import helmclient "github.com/mittwald/go-helm-client"
// TODO:
/*
	1. In the service dev, move helm3 as a parallel section with "deployment", "clusterDeployment" - this requires changes in exchange
	2. In node status: currently uses "operatorStatus" field to store the release resource status, may need a separate section for helm resourse - this requires changes in exchange
*/

const HELM3_DRIVER = "HELM_DRIVER"

type Helm3ClientSet struct {
}

type Helm3Client struct {
	KubeClientSet  *KubeClient
	Helm3ClientSet *Helm3ClientSet
}

func NewHelm3Client() (*Helm3Client, error) {
	k8sclientset, err := NewKubeClient()
	if err != nil {
		return nil, err
	}

	helm3clientset := &Helm3ClientSet{}

	Helm3Client := Helm3Client{KubeClientSet: k8sclientset, Helm3ClientSet: helm3clientset}
	return &Helm3Client, nil
}

func (h Helm3Client) InstallChart(b64Package string, releaseName string, namespace string, envVars map[string]string, fssAuthFilePath string, fssCertFilePath string, secretsMap map[string]string, agId string) error {
	glog.V(3).Infof(h3wlog(fmt.Sprintf("lily - Installing helm3 chart at: %v, release: %v, namespace: %v, agreementId: %v", b64Package, releaseName, namespace, agId)))
	var fileName string
	fileName, err := ConvertB64StringToFile(b64Package, agId)
	if err != nil {
		return fmt.Errorf("error converting Helm3 package to file: %v", err)
	}
	glog.V(3).Infof(h3wlog(fmt.Sprintf("lily - Decoded Helm package to file: %v", fileName)))

	chart, err := loader.Load(fileName)
	if err != nil {
		return err
	}

	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(
		&genericclioptions.ConfigFlags{
			Namespace: &namespace,
		},
		namespace,
		os.Getenv(HELM3_DRIVER),
		log.Default().Printf,
	); err != nil {
		return err
	}

	client := action.NewInstall(actionConfig)
	client.ReleaseName = releaseName
	client.Namespace = namespace
	client.DryRun = true

	// If namespace != agent namespace && agent is cluster scope, then create namespace
	if err := h.KubeClientSet.InstallNamespace(namespace); err != nil {
		return err
	}

	// dry run to get the k8s manifests
	var vals map[string]interface{}
	var dryRunRelease *release.Release
	if dryRunRelease, err = client.RunWithContext(context.TODO(), chart, vals); err != nil {
		return err
	} else if dryRunRelease == nil {
		return fmt.Errorf("failed ot dry run release %v, nil release returned", releaseName)
	}

	glog.V(3).Infof(h3wlog(fmt.Sprintf("lily - dryRunRelease.Manifest is %v", dryRunRelease.Manifest)))

	deploymentManifestObjs, err := GetK8sObjectFromYaml(dryRunRelease.Manifest, nil)
	if err != nil {
		return fmt.Errorf("failed ot get deployment manifest from release %v, error: %v", releaseName, err)
	}

	// Need to let the deployment template reference those chartValue
	checkedTmplFile := []*h3chart.File{}
	otherTemplateFiles := []*h3chart.File{}
	templates := chart.Templates
	for _, templatefile := range templates {
		glog.V(3).Infof(h3wlog(fmt.Sprintf("lily - template file name is %v", templatefile.Name)))
		if strings.Contains(templatefile.Name, "deployment.yml") || strings.Contains(templatefile.Name, "deployment.yaml") || strings.Contains(templatefile.Name, "deployment") {
			// this is the file we want to read in
			glog.V(3).Infof(h3wlog(fmt.Sprintf("lily - get deployment template file %v, now check deployment manifest", templatefile.Name)))
			deploymentObject, ok := deploymentManifestObjs[templatefile.Name]
			if !ok {
				// should not reach here
				glog.Errorf(h3wlog(fmt.Sprintf("failed to find deployment manifest files %v: %v", templatefile.Name, err)))
				continue
			}
			glog.V(3).Infof(h3wlog(fmt.Sprintf("lily - find deployment object %v", deploymentObject)))
			deploymentWithHzn, err := deploymentObject.AddHznToDeployment(h.KubeClientSet, releaseName, namespace, envVars, fssAuthFilePath, fssCertFilePath, secretsMap, agId)
			if err != nil {
				return fmt.Errorf("failed to update deployment under release %v, error: %v", releaseName, err)
			}

			dbytes, err := cutil.ConvertK8sDeploymentToYaml(*deploymentWithHzn)
			if err != nil {
				return fmt.Errorf("failed to convert deployment %v to yaml for release %v, error: %v", deploymentWithHzn.GetName(), releaseName, err)
			}
			templatefile.Data = dbytes

			checkedTmplFile = append(checkedTmplFile, templatefile)
		} else {
			otherTemplateFiles = append(otherTemplateFiles, templatefile)
		}
	}
	glog.V(3).Infof(h3wlog(fmt.Sprintf("lily - checkedTmplFile list: %v", checkedTmplFile)))
	glog.V(3).Infof(h3wlog(fmt.Sprintf("lily - otherTemplateFiles list: %v", otherTemplateFiles)))
	otherTemplateFiles = append(otherTemplateFiles, checkedTmplFile...)
	// add the updated deployment template back to chart
	chart.Templates = otherTemplateFiles

	client.DryRun = false
	if release, err := client.RunWithContext(context.TODO(), chart, nil); err != nil {
		glog.V(3).Infof(h3wlog(fmt.Sprintf("lily - failed to install, error %v", err)))
		return err
	} else if release == nil {
		return fmt.Errorf("failed ot install release %v, nil release returned", releaseName)
	}

	return nil
}

func (h Helm3Client) UninstallChart(releaseName string, namespace string, agId string) error {
	glog.V(3).Infof(h3wlog(fmt.Sprintf("lily - Uninstalling helm3 release: %v, namespace: %v, agreementId: %v", releaseName, namespace, agId)))
	// 1. uninstall release
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(
		&genericclioptions.ConfigFlags{
			Namespace: &namespace,
		},
		namespace,
		os.Getenv(HELM3_DRIVER),
		log.Default().Printf,
	); err != nil {
		return err
	}

	uninstallClient := action.NewUninstall(actionConfig)
	//uninstallClient.DeletionPropagation = "foreground" // "background" or "orphan"
	if result, err := uninstallClient.Run(releaseName); err != nil {
		return err
	} else if result != nil {
		glog.V(3).Infof(h3wlog(fmt.Sprintf("lily - helm3 release uninstall result: %+v\n", *result.Release)))
	}
	glog.V(3).Infof(h3wlog(fmt.Sprintf("lily - Helm3 release: %v, namespace: %v, agreementId: %v is uninstalled", releaseName, namespace, agId)))

	// 2. uninstall all the hzn k8s object (configmap, ess secrets, mms pvc, service secrets)
	glog.V(3).Infof(h3wlog(fmt.Sprintf("lily - Now uninstalling hzn k8s object related to helm3 release: %v, namespace: %v, agreementId: %v", releaseName, namespace, agId)))
	h.UninstallK8sObjects(releaseName, namespace, agId)

	// 3. uninstall namespace if namespace != agent namespace
	glog.V(3).Infof(h3wlog(fmt.Sprintf("lily - Now attampting to uninstall namespace of helm3 release: %v, namespace: %v, agreementId: %v", releaseName, namespace, agId)))
	h.KubeClientSet.UninstallNamespace(namespace)
	return nil
}

func (h Helm3Client) UninstallK8sObjects(releaseName string, namespace string, agId string) {
	glog.V(3).Infof(h3wlog(fmt.Sprintf("for helm3 release %v, deleting config map for agreement %v in namespace %v", releaseName, agId, namespace)))
	if err := h.KubeClientSet.DeleteConfigMap(agId, namespace); err != nil {
		glog.Errorf(h3wlog(err.Error()))
	}

	glog.V(3).Infof(h3wlog(fmt.Sprintf("for helm3 release %v, deleting ess auth secret for agreement %v in namespace %v", releaseName, agId, namespace)))
	if err := h.KubeClientSet.DeleteESSAuthSecrets(agId, namespace); err != nil {
		glog.Errorf(h3wlog(err.Error()))
	}

	glog.V(3).Infof(h3wlog(fmt.Sprintf("for helm3 release %v, deleting ess cert secret for agreement %v in namespace %v", releaseName, agId, namespace)))
	if err := h.KubeClientSet.DeleteESSCertSecrets(agId, namespace); err != nil {
		glog.Errorf(h3wlog(err.Error()))
	}

	glog.V(3).Infof(h3wlog(fmt.Sprintf("for helm3 release %v, deleting service secret for agreement %v in namespace %v", releaseName, agId, namespace)))
	if err := h.KubeClientSet.DeleteK8SSecrets(agId, namespace); err != nil {
		glog.Errorf(h3wlog(err.Error()))
	}

	if namespace != cutil.GetClusterNamespace() {
		glog.V(3).Infof(h3wlog(fmt.Sprintf("for helm3 release %v, deleting namespace: %v", releaseName, namespace)))
		h.KubeClientSet.UninstallNamespace(namespace)
	}
}

func (h Helm3Client) ReleaseStatus(releaseName string, namespace string) (*release.Release, error) {
	glog.V(3).Infof(h3wlog(fmt.Sprintf("lily - get helm3 release status %v in namespace %v", releaseName, namespace)))

	// 1. helm release status
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(
		&genericclioptions.ConfigFlags{
			Namespace: &namespace,
		},
		namespace,
		os.Getenv(HELM3_DRIVER),
		log.Default().Printf,
	); err != nil {
		return nil, err
	}

	statusClient := action.NewStatus(actionConfig)
	statusClient.ShowResources = true

	if release, err := statusClient.Run(releaseName); err != nil {
		return nil, err
	} else if release == nil {
		return nil, fmt.Errorf("unable to list status for nil release: %v", releaseName)
	} else {
		return release, nil
	}
}

// return the helm resource status which is related to this release
func (h Helm3Client) ResourceStatus(releaseName string, namespace string) ([]cutil.PodContainerStatus, interface{}, error) {
	glog.V(3).Infof(h3wlog(fmt.Sprintf("lily - get helm3 status %v in namespace %v", releaseName, namespace)))
	result := []cutil.PodContainerStatus{}
	var releaseResources map[string][]runtime.Object
	// 1. helm release status
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(
		&genericclioptions.ConfigFlags{
			Namespace: &namespace,
		},
		namespace,
		os.Getenv(HELM3_DRIVER),
		log.Default().Printf,
	); err != nil {
		return result, nil, err
	}

	statusClient := action.NewStatus(actionConfig)
	statusClient.ShowResources = true

	if release, err := statusClient.Run(releaseName); err != nil {
		return result, nil, err
	} else if release == nil || release.Info == nil {
		return result, nil, fmt.Errorf("unable to list status for nil release (or nil release info): %v", releaseName)
	} else {
		releaseResources = release.Info.Resources
	}

	return result, releaseResources, nil
}

func (h Helm3Client) UpdateServiceSecret(releaseName string, agId string, namespace string, updatedSecrets []persistence.PersistedServiceSecret) error {
	updatedSecretsMap := make(map[string]string, 0)
	for _, pss := range updatedSecrets {
		secName := pss.SvcSecretName
		secValue := pss.SvcSecretValue
		updatedSecretsMap[secName] = secValue
	}

	return h.KubeClientSet.UpdateK8SSecrets(updatedSecretsMap, agId, namespace)
}

func ConvertB64StringToFile(b64Package string, agreementId string) (string, error) {
	if sDec, err := base64.StdEncoding.DecodeString(b64Package); err != nil {
		return "", err
	} else if f, err := os.OpenFile(fmt.Sprintf("./chart-%v.tar.gzip", agreementId), os.O_CREATE|os.O_RDWR, os.FileMode(0600)); err != nil {
		return "", err
	} else {
		defer cutil.CloseFileLogError(f)
		num, err := f.Write(sDec)
		if err != nil {
			return "", err
		}
		glog.V(3).Infof(h3wlog(fmt.Sprintf("Wrote %v bytes to temp Helm3 package to file: %v", num, f.Name())))
		return f.Name(), nil
	}
}

type ManifestYaml struct {
	FileName string
	Body     string
}

// Convert the given yaml files into k8s api objects
func GetK8sObjectFromYaml(releaseManifest string, sch *runtime.Scheme) (map[string]DeploymentObject, error) {
	retObjects := make(map[string]DeploymentObject, 0)

	if sch == nil {
		sch = runtime.NewScheme()
	}

	// This is required to allow the schema to recognize custom resource definition types
	_ = v1beta1scheme.AddToScheme(sch)
	_ = v1scheme.AddToScheme(sch)
	_ = corev1.AddToScheme(sch)
	_ = appsv1.AddToScheme(sch)
	_ = rbacv1.AddToScheme(sch)
	_ = scheme.AddToScheme(sch)
	_ = olmv1alpha1scheme.AddToScheme(sch)
	_ = olmv1scheme.AddToScheme(sch)

	// multiple yaml files can be in one file separated by '---'
	// these are split here and rejoined with the single files
	indivYamls := []ManifestYaml{}

	if multFiles := strings.Split(releaseManifest, "---"); len(multFiles) > 0 {
		for _, indivYaml := range multFiles {
			if strings.TrimSpace(indivYaml) != "" {
				// # Source: nginx-chart/templates/deployment.yaml
				manifestNameRegex := regexp.MustCompile("# Source: [^/]+/(.+)")
				submatch := manifestNameRegex.FindStringSubmatch(indivYaml)
				for i, ele := range submatch {
					glog.Infof(h3wlog(fmt.Sprintf("submatch[%v] is: %v", i, ele))) // submatch[0] is: # Source: nginx-chart/templates/deployment.yaml
					// submatch[1] is: templates/deployment.yaml
				}
				glog.Infof(h3wlog(fmt.Sprintf("submatch is: %v", submatch))) //submatch is: [# Source: nginx-chart/templates/service.yaml templates/service.yaml]
				indivYamls = append(indivYamls, ManifestYaml{FileName: submatch[1], Body: indivYaml})
			}
		}
	}

	for _, fileStr := range indivYamls {
		decode := serializer.NewCodecFactory(sch).UniversalDecoder(v1beta1scheme.SchemeGroupVersion, v1scheme.SchemeGroupVersion, rbacv1.SchemeGroupVersion, appsv1.SchemeGroupVersion, corev1.SchemeGroupVersion, olmv1alpha1scheme.SchemeGroupVersion, olmv1scheme.SchemeGroupVersion).Decode
		obj, gvk, err := decode([]byte(fileStr.Body), nil, nil)

		if err != nil {
			return retObjects, err
		} else if gvk.Kind == cutil.K8S_DEPLOYMENT_TYPE {
			//newObj := cutil.APIObjects{Type: gvk, Object: obj}
			if typedDeployment, ok := obj.(*appsv1.Deployment); ok {
				retObjects[fileStr.FileName] = DeploymentObject{Deployment: typedDeployment}
			}

		}
	}

	return retObjects, nil
}
