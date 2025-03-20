package cutil

import (
	"archive/tar"
	"context"
	"fmt"
	"github.com/golang/glog"
	olmv1scheme "github.com/operator-framework/api/pkg/operators/v1"
	olmv1alpha1scheme "github.com/operator-framework/api/pkg/operators/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	v1scheme "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	v1beta1scheme "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"math"
	"os"
	"sigs.k8s.io/yaml"
	"strings"
)

const (
	DEFAULT_ANAX_NAMESPACE = "openhorizon-agent"
	// Name for the env var config map. Only characters allowed: [a-z] "." and "-"
	HZN_ENV_VARS = "hzn-env-vars"
	// Variable that contains the name of the config map
	HZN_ENV_KEY = "HZN_ENV_VARS"
	// Name for the k8s secrets that contains service secrets. Only characters allowed: [a-z] "." and "-"
	HZN_SERVICE_SECRETS = "hzn-service-secrets"

	SECRETS_VOLUME_NAME = "service-secrets-vol"

	AGENT_PVC_NAME       = "openhorizon-agent-pvc"
	MMS_VOLUME_NAME      = "mms-shared-storage"
	MMS_AUTH_VOLUME_NAME = "mms-auth-volume"
	MMS_AUTH_MOUNT_PATH  = "/ess-auth"
	MMS_CERT_VOLUME_NAME = "mms-cert-volume"
	MMS_CERT_MOUNT_PATH  = "/ess-cert"

	K8S_CLUSTER_ROLE_TYPE          = "ClusterRole"
	K8S_CLUSTER_ROLEBINDING_TYPE   = "ClusterRoleBinding"
	K8S_ROLE_TYPE                  = "Role"
	K8S_ROLEBINDING_TYPE           = "RoleBinding"
	K8S_DEPLOYMENT_TYPE            = "Deployment"
	K8S_SERVICEACCOUNT_TYPE        = "ServiceAccount"
	K8S_CRD_TYPE                   = "CustomResourceDefinition"
	K8S_NAMESPACE_TYPE             = "Namespace"
	K8S_SECRET_TYPE                = "Secret"
	K8S_UNSTRUCTURED_TYPE          = "Unstructured"
	K8S_POD_TYPE                   = "Pod"
	K8S_OLM_OPERATOR_GROUP_TYPE    = "OperatorGroup"
	K8S_MMS_SHARED_PVC_NAME        = "mms-shared-storage-pvc"
	STORAGE_CLASS_USERINPUT_NAME   = "MMS_K8S_STORAGE_CLASS"
	PVC_SIZE_USERINPUT_NAME        = "MMS_K8S_STORAGE_SIZE"
	PVC_ACCESS_MODE_USERINPUT_NAME = "MMS_K8S_PVC_ACCESS_MODE"
	DEFAULT_PVC_SIZE_IN_STRING     = "10"
)

func NewKubeConfig() (*rest.Config, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("Failed to get cluster config information: %v", err)
	}
	return config, nil
}

func NewKubeClient() (*kubernetes.Clientset, error) {
	config, err := NewKubeConfig()
	if err != nil {
		return nil, err
	}
	clientset, _ := kubernetes.NewForConfig(config)
	return clientset, nil
}

// NewDynamicKubeClient returns a kube client that interacts with unstructured.Unstructured type objects
func NewDynamicKubeClient() (dynamic.Interface, error) {
	config, err := NewKubeConfig()
	if err != nil {
		return nil, err
	}
	clientset, _ := dynamic.NewForConfig(config)
	return clientset, nil
}

// GetClusterCountInfo returns the cluster's available memory, total memory, cpu count, arch, kube version, cluster namespace, agent scope or an error if it cannot get the client
func GetClusterCountInfo() (float64, float64, float64, string, string, string, bool, error) {
	client, err := NewKubeClient()
	if err != nil {
		return 0, 0, 1, "", "", "", false, fmt.Errorf("Failed to get kube client for introspecting cluster properties. Proceding with default values. %v", err)
	}
	versionObj, err := client.Discovery().ServerVersion()
	if err != nil {
		glog.Warningf("Failed to get kubernetes server version: %v", err)
	}
	version := ""
	if versionObj != nil {
		version = versionObj.GitVersion
	}

	// get kube namespace
	ns := GetClusterNamespace()
	isNamespaceScoped := IsNamespaceScoped()

	availMem := float64(0)
	totalMem := float64(0)
	cpu := float64(0)
	arch := ""
	nodes, err := client.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return 0, 0, 0, "", "", "", false, err
	}

	for _, node := range nodes.Items {
		if arch == "" {
			arch = node.Status.NodeInfo.Architecture
		}
		availMem += FloatFromQuantity(node.Status.Allocatable.Memory()) / 1000000
		totalMem += FloatFromQuantity(node.Status.Capacity.Memory()) / 1000000
		cpu += FloatFromQuantity(node.Status.Capacity.Cpu())
	}

	return math.Round(availMem), math.Round(totalMem), cpu, arch, version, ns, isNamespaceScoped, nil
}

// FloatFromQuantity returns a float64 with the value of the given quantity type
func FloatFromQuantity(quantVal *resource.Quantity) float64 {
	if intVal, ok := quantVal.AsInt64(); ok {
		return float64(intVal)
	}
	decVal := quantVal.AsDec()
	unscaledVal := decVal.UnscaledBig().Int64()
	scale := decVal.Scale()
	floatVal := float64(unscaledVal) * math.Pow10(-1*int(scale))
	return floatVal
}

func GetClusterNamespace() string {
	// get kube namespace
	ns := os.Getenv("AGENT_NAMESPACE")
	if ns == "" {
		ns = "openhorizon-agent"
	}

	return ns
}

func IsNamespaceScoped() bool {
	isNamespaceScoped := os.Getenv("HZN_NAMESPACE_SCOPED")
	return isNamespaceScoped == "true"
}

// pvc name: openhorizon-agent-pvc
func GetAgentPVCInfo() (string, []v1.PersistentVolumeAccessMode, error) {
	client, err := NewKubeClient()
	if err != nil {
		return "", []v1.PersistentVolumeAccessMode{}, err
	}

	agentNamespace := GetClusterNamespace()
	if agentPVC, err := client.CoreV1().PersistentVolumeClaims(agentNamespace).Get(context.Background(), AGENT_PVC_NAME, metav1.GetOptions{}); err != nil {
		return "", []v1.PersistentVolumeAccessMode{}, err
	} else {
		scName := agentPVC.Spec.StorageClassName
		accessMode := agentPVC.Spec.AccessModes
		return *scName, accessMode, nil
	}
}

// Intermediate state for the objects used for k8s api objects that haven't had their exact type asserted yet
type APIObjects struct {
	Type   *schema.GroupVersionKind
	Object interface{}
}

// Intermediate state used for after the objects have been read from the deployment but not converted to k8s objects yet
type YamlFile struct {
	Header tar.Header
	Body   string
}

type PodContainerStatus struct {
	Name        string
	Image       string
	CreatedTime int64
	State       string
}

func IsBaseK8sType(validKind []string, kind string) bool {
	return SliceContains(validKind, kind)
}

func IsDangerType(dangerTypes []string, kind string) bool {
	return SliceContains(dangerTypes, kind)
}

// Convert the given yaml files into k8s api objects
func GetK8sObjectFromYaml(yamlFiles []YamlFile, sch *runtime.Scheme, validKind []string, dangerTypes []string) ([]APIObjects, []YamlFile, error) {
	retObjects := []APIObjects{}
	customResources := []YamlFile{}

	if sch == nil {
		sch = runtime.NewScheme()
	}

	// This is required to allow the schema to recognize custom resource definition types
	_ = v1beta1scheme.AddToScheme(sch)
	_ = v1scheme.AddToScheme(sch)
	_ = scheme.AddToScheme(sch)
	_ = olmv1alpha1scheme.AddToScheme(sch)
	_ = olmv1scheme.AddToScheme(sch)

	// multiple yaml files can be in one file separated by '---'
	// these are split here and rejoined with the single files
	indivYamls := []YamlFile{}
	for _, file := range yamlFiles {
		if multFiles := strings.Split(file.Body, "---"); len(multFiles) > 1 {
			for _, indivYaml := range multFiles {
				if strings.TrimSpace(indivYaml) != "" {
					indivYamls = append(indivYamls, YamlFile{Body: indivYaml})
				}
			}
		} else {
			indivYamls = append(indivYamls, file)
		}
	}

	for _, fileStr := range indivYamls {
		decode := serializer.NewCodecFactory(sch).UniversalDecoder(v1beta1scheme.SchemeGroupVersion, v1scheme.SchemeGroupVersion, rbacv1.SchemeGroupVersion, appsv1.SchemeGroupVersion, v1.SchemeGroupVersion, olmv1alpha1scheme.SchemeGroupVersion, olmv1scheme.SchemeGroupVersion).Decode
		obj, gvk, err := decode([]byte(fileStr.Body), nil, nil)

		if err != nil {
			customResources = append(customResources, fileStr)
		} else if IsBaseK8sType(validKind, gvk.Kind) {
			newObj := APIObjects{Type: gvk, Object: obj}
			retObjects = append(retObjects, newObj)
		} else if IsDangerType(dangerTypes, gvk.Kind) {
			// the scheme has recognized this type but does not provide the function for converting it to an unstructured object. skip this one to avoid a panic.
			glog.Errorf("Skipping unsupported kind %v", gvk.Kind)
		} else {
			newUnstructObj := unstructured.Unstructured{}
			err = sch.Convert(obj, &newUnstructObj, conversion.Meta{})
			if err != nil {
				glog.Errorf("Err converting object to unstructured: %v", err)
			}
			newObj := APIObjects{Type: gvk, Object: &newUnstructObj}
			retObjects = append(retObjects, newObj)
		}
	}

	return retObjects, customResources, nil
}

func ConvertK8sDeploymentToYaml(k8sDeployment appsv1.Deployment) ([]byte, error) {
	yamlBytes, err := yaml.Marshal(k8sDeployment)
	if err != nil {
		return []byte{}, err
	}
	return yamlBytes, nil
}

func ConvertK8sSecretToYaml(k8sSecret v1.Secret) ([]byte, error) {
	yamlBytes, err := yaml.Marshal(k8sSecret)
	if err != nil {
		return []byte{}, err
	}
	return yamlBytes, nil
}
