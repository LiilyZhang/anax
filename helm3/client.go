package helm3

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/golang/glog"
	"github.com/open-horizon/anax/cutil"
	"github.com/open-horizon/anax/exchange"
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

	allYamls := GetAllYamls(dryRunRelease.Manifest)

	deploymentManifestObjs, err := GetK8sObjectFromYaml(allYamls, nil, cutil.K8S_DEPLOYMENT_TYPE)
	if err != nil {
		return fmt.Errorf("failed ot get deployment manifests from release %v, error: %v", releaseName, err)
	}
	secretsManifestObjs, err := GetK8sObjectFromYaml(allYamls, nil, cutil.K8S_SECRET_TYPE)
	if err != nil {
		return fmt.Errorf("failed ot get secret manifests from release %v, error: %v", releaseName, err)
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
			deploymentObjectsInterface, ok := deploymentManifestObjs[templatefile.Name]
			if !ok {
				// should not reach here
				glog.Errorf(h3wlog(fmt.Sprintf("failed to find deployment manifest files %v: %v", templatefile.Name, err)))
				continue
			}
			glog.V(3).Infof(h3wlog(fmt.Sprintf("lily - find deployment objects %v", deploymentObjectsInterface)))
			newTemplateData := []byte{}
			for _, deploymentObject := range deploymentObjectsInterface {
				if dp, ok := deploymentObject.(DeploymentObject); !ok {
					return fmt.Errorf("failed to convert interface %v to deployment object for release %v", deploymentObject, releaseName)
				} else {
					deploymentWithHzn, err := dp.AddHznToDeployment(h.KubeClientSet, releaseName, namespace, envVars, fssAuthFilePath, fssCertFilePath, secretsMap, agId)
					if err != nil {
						return fmt.Errorf("failed to update deployment under release %v, error: %v", releaseName, err)
					}

					dbytes, err := cutil.ConvertK8sDeploymentToYaml(*deploymentWithHzn)
					if err != nil {
						return fmt.Errorf("failed to convert deployment %v to yaml for release %v, error: %v", deploymentWithHzn.GetName(), releaseName, err)
					}
					newTemplateData = append(newTemplateData, dbytes...)
					//templatefile.Data = append(templatefile.Data, dbytes...)
				}
			}
			templatefile.Data = newTemplateData
			checkedTmplFile = append(checkedTmplFile, templatefile)
		} else if strings.Contains(templatefile.Name, "secret.yml") || strings.Contains(templatefile.Name, "secret.yaml") || strings.Contains(templatefile.Name, "secrets.yml") || strings.Contains(templatefile.Name, "secrets.yaml") {
			// if this is a secret yaml template, need to check the secret key-value pairs. If the secret data key is in the keys of secretsMap, replace the value with the value in secretsMap
			glog.V(3).Infof(h3wlog(fmt.Sprintf("lily - get secrets template file %v, now check secret data from manifest", templatefile.Name)))
			secretObjectsInterface, ok := secretsManifestObjs[templatefile.Name]
			if !ok {
				// should not reach here
				glog.Errorf(h3wlog(fmt.Sprintf("failed to find secret manifest files %v: %v", templatefile.Name, err)))
				continue
			}

			glog.V(3).Infof(h3wlog(fmt.Sprintf("lily - find secrets objects interface %v", secretObjectsInterface)))
			newTemplateData := []byte{}
			for _, secretObject := range secretObjectsInterface {
				if scr, ok := secretObject.(SecretObject); !ok || scr.K8sSecret == nil {
					return fmt.Errorf("failed to convert interface %v to secret object for release %v", secretObject, releaseName)
				} else {
					dataMap := scr.K8sSecret.Data
					for dataKey, _ := range dataMap {
						if secretValueFromAg, ok := secretsMap[dataKey]; ok {
							// the helm secret contains a secretKey which also in the agreement, replace the helm secretValue with value from agreement
							decodedSecValueBytes, err := base64.StdEncoding.DecodeString(secretValueFromAg)
							if err != nil {
								return fmt.Errorf("failed to update secret value for %v in %v under release %v, error: %v", dataKey, scr.K8sSecret.Name, releaseName, err)
							}
							dataMap[dataKey] = decodedSecValueBytes
						}
					}
					scr.K8sSecret.Data = dataMap
					sbytes, err := cutil.ConvertK8sSecretToYaml(*scr.K8sSecret)
					if err != nil {
						return fmt.Errorf("failed to convert secret %v to yaml for release %v, error: %v", scr.K8sSecret.GetName(), releaseName, err)
					}
					newTemplateData = append(newTemplateData, sbytes...)
				}
			}
			templatefile.Data = newTemplateData
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

// type Helm3Status struct {
// 	ReleaseStatus         exchange.HelmStatus
// 	ReleaseResourceStatus interface{}
// }

// return the helm resource status which is related to this release
func (h Helm3Client) ResourceStatus(releaseName string, namespace string) (exchange.HelmStatus, error) {
	glog.V(3).Infof(h3wlog(fmt.Sprintf("lily - get helm3 status %v in namespace %v", releaseName, namespace)))
	var result exchange.HelmStatus
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
		return result, err
	}

	statusClient := action.NewStatus(actionConfig)
	statusClient.ShowResources = true

	if release, err := statusClient.Run(releaseName); err != nil {
		return result, err
	} else if release == nil || release.Info == nil {
		return result, fmt.Errorf("unable to list status for nil release (or nil release info): %v", releaseName)
	} else {
		releaseResources = release.Info.Resources

		result = exchange.HelmStatus{
			ReleaseName:           releaseName,
			Namespace:             namespace,
			Status:                string(release.Info.Status),
			ChartVersion:          release.Chart.Metadata.Version,
			AppVersion:            release.Chart.AppVersion(),
			Revision:              release.Version,
			FirstDeployed:         release.Info.FirstDeployed.Unix(),
			LastDeployed:          release.Info.LastDeployed.Unix(),
			ReleaseResourceStatus: releaseResources,
		}

	}

	return result, nil
}

func (h Helm3Client) UpdateServiceSecret(releaseName string, agId string, namespace string, updatedSecrets []persistence.PersistedServiceSecret) error {
	updatedSecretsMap := make(map[string]string, 0)
	for _, pss := range updatedSecrets {
		secName := pss.SvcSecretName
		secValue := pss.SvcSecretValue
		updatedSecretsMap[secName] = secValue
	}

	// Update hzn-service-secrets-<agId> secret, this secret contains all the service secret defined in the service/deployment policy
	if err := h.KubeClientSet.UpdateK8SSecrets(updatedSecretsMap, agId, namespace); err != nil {
		return err
	}

	if err := h.updateSecretValueInHelm(updatedSecretsMap, agId, namespace); err != nil {
		return err
	}

	return nil
}

func (h Helm3Client) updateSecretValueInHelm(updatedSecretsMap map[string]string, agId string, namespace string) error {
	if allSecrets, err := h.KubeClientSet.ListAllK8SSecrets(agId, namespace); err != nil {
		return err
	} else if len(allSecrets) > 0 {
		for _, s := range allSecrets {
			dataMap := s.Data
			for secretDataKey, _ := range dataMap {
				if updatedSecretValue, ok := updatedSecretsMap[secretDataKey]; ok {
					decodedSecValueBytes, err := base64.StdEncoding.DecodeString(updatedSecretValue)
					if err != nil {
						return fmt.Errorf("failed to update secret value for %v in %v for agreement %v, error: %v", secretDataKey, s.Name, agId, err)
					}
					dataMap[secretDataKey] = decodedSecValueBytes
				}
			}
			s.Data = dataMap
			if err := h.KubeClientSet.UpdateK8SSecret(&s, agId, namespace); err != nil {
				return err
			}
		}
	}
	return nil
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

func GetAllYamls(releaseManifest string) []ManifestYaml {
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
	return indivYamls
}

// Convert the given yaml files into k8s api objects
func GetK8sObjectFromYaml(indivYamls []ManifestYaml, sch *runtime.Scheme, kind string) (map[string][]interface{}, error) {
	retObjects := make(map[string][]interface{}, 0)

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

	for _, fileStr := range indivYamls {
		decode := serializer.NewCodecFactory(sch).UniversalDecoder(v1beta1scheme.SchemeGroupVersion, v1scheme.SchemeGroupVersion, rbacv1.SchemeGroupVersion, appsv1.SchemeGroupVersion, corev1.SchemeGroupVersion, olmv1alpha1scheme.SchemeGroupVersion, olmv1scheme.SchemeGroupVersion).Decode
		obj, gvk, err := decode([]byte(fileStr.Body), nil, nil)

		if err != nil {
			return retObjects, err
		} else if kind == cutil.K8S_DEPLOYMENT_TYPE && kind == gvk.Kind {
			// passed in kind is cutil.K8S_DEPLOYMENT_TYPE
			if typedDeployment, ok := obj.(*appsv1.Deployment); ok {
				retObjects[fileStr.FileName] = append(retObjects[fileStr.FileName], DeploymentObject{Deployment: typedDeployment})
			}

		} else if kind == cutil.K8S_SECRET_TYPE && kind == gvk.Kind {
			glog.Infof(h3wlog(fmt.Sprintf("try to parse k8s secret object, gvk.Kind is: %v", gvk.Kind)))
			if typedSecret, ok := obj.(*corev1.Secret); ok {
				retObjects[fileStr.FileName] = append(retObjects[fileStr.FileName], SecretObject{K8sSecret: typedSecret})
			}
		}
	}

	return retObjects, nil
}
