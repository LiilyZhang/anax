package helm3

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/golang/glog"
	"github.com/open-horizon/anax/config"
	"github.com/open-horizon/anax/cutil"
	"github.com/open-horizon/anax/kube_operator"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"os"
)

// Client to interact with all standard k8s objects
type KubeClient struct {
	Client *kubernetes.Clientset
}

func NewKubeClient() (*KubeClient, error) {
	clientset, err := cutil.NewKubeClient()
	if err != nil {
		return nil, err
	}
	return &KubeClient{Client: clientset}, nil
}

var (
	accessModeMap = map[string]corev1.PersistentVolumeAccessMode{
		"ReadWriteOnce": corev1.ReadWriteOnce,
		"ReadWriteMany": corev1.ReadWriteMany,
	}
)

func (c KubeClient) InstallNamespace(namespace string) error {
	agentNamespace := cutil.GetClusterNamespace()
	isNamespaceScoped := cutil.IsNamespaceScoped()

	if isNamespaceScoped && (namespace != agentNamespace) {
		return fmt.Errorf("namespace-scoped agent can not create other namespace %v for service to deploy", namespace)
	} else if namespace == agentNamespace {
		return nil
	} else {
		// cluster-scoped agent wants to deploy service in a different namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}

		_, err := c.Client.CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})
		if err != nil && !errors.IsAlreadyExists(err) {
			return err
		}

	}
	return nil
}

func (c KubeClient) UninstallNamespace(namespace string) {
	agentNamespace := cutil.GetClusterNamespace()
	isNamespaceScoped := cutil.IsNamespaceScoped()
	if isNamespaceScoped && (namespace != agentNamespace) {
		glog.Errorf("namespace-scoped agent can not uninstall other namespace %v for service to deploy", namespace)
		return
	} else if namespace == agentNamespace {
		glog.V(3).Infof(h3wlog("helme service namespace is same as agent's namespace, skip namespace uninstall"))
		return
	} else {
		glog.V(3).Infof(h3wlog(fmt.Sprintf("deleting namespace %v", namespace)))
		err := c.Client.CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
		if err != nil {
			glog.Errorf(h3wlog(fmt.Sprintf("unable to delete namespace %s. Error was: %v, now force deleting namespace", namespace, err)))
			gracePeriodSeconds := int64(0)
			if err = c.Client.CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{GracePeriodSeconds: &gracePeriodSeconds}); err != nil {
				glog.Errorf(h3wlog(fmt.Sprintf("unable to force delete namespace %s. Error: %v", namespace, err)))
			}
		}

	}
}

// HZN_NODE_ID=agent-in-kube
// HZN_AGREEMENTID=87416db1e95b9f048f7d360bff45904b262ac6bad73dd447cdae829b3fde7303
// HZN_PATTERN=
// HZN_RAM=8323
// HZN_ORGANIZATION=userdev
// HZN_CPUS=4
// HZN_ESS_API_PORT=8443
// HZN_DEVICE_ID=agent-in-kube
// HZN_ARCH=amd64
// HZN_AGENT_STORAGE_CLASS_NAME=microk8s-hostpath
// HZN_PRIVILEGED=true
// HZN_ESS_AUTH=ess-auth-87416db1e95b9f048f7d360bff45904b262ac6bad73dd447cdae829b3fde7303
// HZN_ESS_API_PROTOCOL=secure
// HZN_ESS_API_ADDRESS=agent-service.agent-namespace.svc.cluster.local
// HZN_AGENT_NAMESPACE=agent-namespace
// HZN_HOST_IPS=127.0.0.1,10.1.90.89
// HZN_ESS_CERT=ess-cert-87416db1e95b9f048f7d360bff45904b262ac6bad73dd447cdae829b3fde7303
// HZN_EXCHANGE_URL=http://192.168.176.6:8080/v1/

// CreateConfigMap will create a config map with the provided environment variable map
func (c KubeClient) CreateConfigMap(envVars map[string]string, agId string, namespace string) (string, error) {
	// a userinput with an empty string for the name will cause an error. need to remove before creating the configmap
	for varName, varVal := range envVars {
		if varName == "" {
			glog.Errorf("Omitting userinput with empty name and value: %v", varVal)
		}
		delete(envVars, "")
	}
	// hzn-env-vars-<agId>
	hznEnvConfigMap := corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("%s-%s", cutil.HZN_ENV_VARS, agId)}, Data: envVars}
	res, err := c.Client.CoreV1().ConfigMaps(namespace).Create(context.Background(), &hznEnvConfigMap, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create config map for %s: %v", agId, err)
	}
	return res.ObjectMeta.Name, nil
}

// DeleteConfigMap will delete the config map with the provided name
func (c KubeClient) DeleteConfigMap(agId string, namespace string) error {
	// hzn-env-vars-<agId>
	hznEnvConfigmapName := fmt.Sprintf("%s-%s", cutil.HZN_ENV_VARS, agId)
	err := c.Client.CoreV1().ConfigMaps(namespace).Delete(context.Background(), hznEnvConfigmapName, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete config map for %s: %v", agId, err)
	}
	return nil
}

// CreateESSSecret will create a k8s secrets object from the ess auth file
func (c KubeClient) CreateESSAuthSecrets(fssAuthFilePath string, agId string, namespace string) (string, error) {
	if essAuthBytes, err := os.ReadFile(fssAuthFilePath); err != nil {
		return "", err
	} else {
		secretData := make(map[string][]byte)
		secretData[config.HZN_FSS_AUTH_FILE] = essAuthBytes
		fssSecret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%s", config.HZN_FSS_AUTH_PATH, agId), // ess-auth-<agId>
				Namespace: namespace,
			},
			Data: secretData,
		}
		res, err := c.Client.CoreV1().Secrets(namespace).Create(context.Background(), &fssSecret, metav1.CreateOptions{})
		if err != nil {
			return "", fmt.Errorf("failed to create ess auth secret for %s: %v", agId, err)
		}
		return res.ObjectMeta.Name, nil
	}

}

func (c KubeClient) DeleteESSAuthSecrets(agId string, namespace string) error {
	essAuthSecretName := fmt.Sprintf("%s-%s", config.HZN_FSS_AUTH_PATH, agId)
	err := c.Client.CoreV1().Secrets(namespace).Delete(context.Background(), essAuthSecretName, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete ess auth secret for %s: %v", agId, err)
	}
	return nil
}

func (c KubeClient) CreateESSCertSecrets(fssCertFilePath string, agId string, namespace string) (string, error) {
	if essCertBytes, err := os.ReadFile(fssCertFilePath); err != nil {
		return "", err
	} else {
		secretData := make(map[string][]byte)
		secretData[config.HZN_FSS_CERT_FILE] = essCertBytes
		certSecret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%s", config.HZN_FSS_CERT_PATH, agId), // ess-cert-<agId>
				Namespace: namespace,
			},
			Data: secretData,
		}

		res, err := c.Client.CoreV1().Secrets(namespace).Create(context.Background(), &certSecret, metav1.CreateOptions{})
		if err != nil && errors.IsAlreadyExists(err) {
			_, err = c.Client.CoreV1().Secrets(namespace).Update(context.Background(), &certSecret, metav1.UpdateOptions{})
		}
		if err != nil {
			return "", fmt.Errorf("failed to create ess cert secret for %s: %v", agId, err)
		}
		return res.ObjectMeta.Name, nil
	}
}

func (c KubeClient) DeleteESSCertSecrets(agId string, namespace string) error {
	essCertSecretName := fmt.Sprintf("%s-%s", config.HZN_FSS_CERT_PATH, agId)
	err := c.Client.CoreV1().Secrets(namespace).Delete(context.Background(), essCertSecretName, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete ess cert secret for %s: %v", agId, err)
	}
	return nil
}

// CreateK8SSecrets will create a k8s secrets object which contains the service secret name and value
func (c KubeClient) CreateK8SSecrets(serviceSecretsMap map[string]string, agId string, namespace string) (string, error) {
	secretsLabel := map[string]string{"name": cutil.HZN_SERVICE_SECRETS}
	hznServiceSecrets := corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("%s-%s", cutil.HZN_SERVICE_SECRETS, agId), Labels: secretsLabel}, StringData: serviceSecretsMap}
	res, err := c.Client.CoreV1().Secrets(namespace).Create(context.Background(), &hznServiceSecrets, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create k8s secrets that contains service secrets for %s: %v", agId, err)
	}
	return res.ObjectMeta.Name, nil
}

// UpdateK8SSecrets will create the k8s secrets object which contains the service secret name and value
func (c KubeClient) UpdateK8SSecrets(serviceSecretsMap map[string]string, agId string, namespace string) error {
	// If len(d.ServiceSecrets) > 0, need to check each entry, and update the value of service secrets entry in the k8s secret (Note: not replace the entire k8s with the d.ServiceSecrets)
	secretsName := fmt.Sprintf("%s-%s", cutil.HZN_SERVICE_SECRETS, agId)
	if len(serviceSecretsMap) > 0 {
		// Update ServiceSecrets

		if k8sSecretObject, err := c.Client.CoreV1().Secrets(namespace).Get(context.Background(), secretsName, metav1.GetOptions{}); err != nil {
			return err
		} else if k8sSecretObject == nil {
			// invalid, return err
			return fmt.Errorf(h3wlog(fmt.Sprintf("Error updating existing service secret %v in namespace: %v, secret doesn't exist", secretsName, namespace)))
		} else {
			// in the dataMap, key is secretName, value is secret value in base64 encoded string
			dataMap := k8sSecretObject.Data
			for secretNameToUpdate, secretValueToUpdate := range serviceSecretsMap {
				if decodedSecValueBytes, err := base64.StdEncoding.DecodeString(secretValueToUpdate); err != nil {
					return err
				} else {
					dataMap[secretNameToUpdate] = decodedSecValueBytes
				}
			}
			k8sSecretObject.Data = dataMap
			updatedSecret, err := c.Client.CoreV1().Secrets(namespace).Update(context.Background(), k8sSecretObject, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
			glog.V(3).Infof(h3wlog(fmt.Sprintf("Service secret %v in namespace %v updated successfully", updatedSecret, namespace)))
		}
	} else {
		glog.V(3).Infof(h3wlog(fmt.Sprintf("No updated service secrets for %v in namespace %v, skip updating", secretsName, namespace)))
	}
	return nil
}

// DeleteK8SSecrets will delete k8s secrets object which contains the service secret name and value
func (c KubeClient) DeleteK8SSecrets(agId string, namespace string) error {
	// delete the secrets contains agreement service vault secrets
	secretsName := fmt.Sprintf("%s-%s", cutil.HZN_SERVICE_SECRETS, agId)
	err := c.Client.CoreV1().Secrets(namespace).Delete(context.Background(), secretsName, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete k8s secrets that contains service secrets for %s: %v", agId, err)
	}
	return nil
}

func (c KubeClient) ListAllK8SSecrets(agId string, namespace string) ([]corev1.Secret, error) {
	secretList, err := c.Client.CoreV1().Secrets(namespace).List(context.Background(), metav1.ListOptions{})
	secrets := []corev1.Secret{}
	if err != nil && !errors.IsNotFound(err) {
		return secrets, fmt.Errorf("failed to list k8s secrets under namespace %v for agreement %s: %v", namespace, agId, err)
	} else if secretList == nil {
		return secrets, nil
	}
	return secretList.Items, nil
}

func (c KubeClient) UpdateK8SSecret(secret *corev1.Secret, agId string, namespace string) error {
	updatedSecret, err := c.Client.CoreV1().Secrets(namespace).Update(context.Background(), secret, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	glog.V(3).Infof(h3wlog(fmt.Sprintf("Service secret %v in namespace %v updated successfully", updatedSecret, namespace)))
	return nil
}

type DeploymentObject struct {
	Deployment *appsv1.Deployment
}

type SecretObject struct {
	K8sSecret *corev1.Secret
}

func (d DeploymentObject) AddHznToDeployment(kubeClientSet *KubeClient, releaseName string, namespace string, envVars map[string]string, fssAuthFilePath string, fssCertFilePath string, secretsMap map[string]string, agId string) (*appsv1.Deployment, error) {
	glog.V(3).Infof(h3wlog(fmt.Sprintf("prepare kubernetes objects for release deployment %v under namespace %s, agreement id %v", releaseName, namespace, agId)))
	cutil.SetESSEnvVarsForClusterAgent(envVars, config.ENVVAR_PREFIX, agId)

	// Create the config map.
	configmapName, err := kubeClientSet.CreateConfigMap(envVars, agId, namespace)
	if err != nil && errors.IsAlreadyExists(err) {
		kubeClientSet.DeleteConfigMap(agId, namespace)
		configmapName, err = kubeClientSet.CreateConfigMap(envVars, agId, namespace)
	}
	if err != nil {
		return nil, err
	}

	// Let the deployment know about the config map
	dWithEnv := addConfigMapVarToDeploymentObject(*d.Deployment, configmapName)

	// create k8s secrets object from ess auth file. d.FssAuthFilePath == "" if kubeworker is updating service vault secret
	if fssAuthFilePath != "" {
		essAuthSecretName, err := kubeClientSet.CreateESSAuthSecrets(fssAuthFilePath, agId, namespace)
		if err != nil && errors.IsAlreadyExists(err) {
			kubeClientSet.DeleteESSAuthSecrets(agId, namespace)
			essAuthSecretName, _ = kubeClientSet.CreateESSAuthSecrets(fssAuthFilePath, agId, namespace)
		}
		dWithEnv = addESSAuthSecretsToDeploymentObject(dWithEnv, essAuthSecretName)
		glog.V(3).Infof(h3wlog(fmt.Sprintf("ess auth secret %v is created under namespace: %v", essAuthSecretName, namespace)))
	}

	if fssCertFilePath != "" {
		essCertSecretName, err := kubeClientSet.CreateESSCertSecrets(fssCertFilePath, agId, namespace)
		if err != nil && errors.IsAlreadyExists(err) {
			kubeClientSet.DeleteESSCertSecrets(agId, namespace)
			essCertSecretName, _ = kubeClientSet.CreateESSCertSecrets(fssCertFilePath, agId, namespace)
		}
		dWithEnv = addESSCertSecretsToDeploymentObject(dWithEnv, essCertSecretName)
		glog.V(3).Infof(h3wlog(fmt.Sprintf("ess cert secret %v is created under namespace: %v", essCertSecretName, namespace)))
	}

	// handle service vault secrets
	if len(secretsMap) > 0 {
		glog.V(3).Infof(h3wlog(fmt.Sprintf("creating k8s secrets for service secret %v", secretsMap)))

		// ServiceSecrets is a map, key is the secret name, value is the base64 encoded string.
		decodedSecrets, err := kube_operator.DecodeServiceSecret(secretsMap)
		if err != nil {
			return nil, err
		}

		secretsName, err := kubeClientSet.CreateK8SSecrets(decodedSecrets, agId, namespace)
		if err != nil && errors.IsAlreadyExists(err) {
			kubeClientSet.DeleteK8SSecrets(agId, namespace)
			secretsName, err = kubeClientSet.CreateK8SSecrets(secretsMap, agId, namespace)
		}
		if err != nil {
			return nil, err
		}

		dWithEnv = addServiceSecretsToDeploymentObject(dWithEnv, secretsName)
	}
	return &dWithEnv, nil
}

// add a reference to the envvar config map to the deployment
func addConfigMapVarToDeploymentObject(deployment appsv1.Deployment, configMapName string) appsv1.Deployment {
	cmr := &corev1.ConfigMapEnvSource{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: configMapName,
		},
	}
	hznEnvVar := corev1.EnvFromSource{ConfigMapRef: cmr}
	i := len(deployment.Spec.Template.Spec.Containers) - 1
	for i >= 0 {
		newEnv := append(deployment.Spec.Template.Spec.Containers[i].EnvFrom, hznEnvVar)
		deployment.Spec.Template.Spec.Containers[i].EnvFrom = newEnv
		i--
	}
	return deployment
}

// add a reference to the secrets service secrets to the deployment
func addServiceSecretsToDeploymentObject(deployment appsv1.Deployment, secretsName string) appsv1.Deployment {
	// Add secrets (secretsName is $HZN_SERVICE_SECRETS-$agId: hzn-service-secrets-12345) as Volume in deployment
	volumeName := cutil.SECRETS_VOLUME_NAME
	// mount the volume to deployment containers
	secretsFilePathInPod := config.HZN_SECRETS_MOUNT
	dp := addSecretsToDeploymentObject(deployment, secretsName, volumeName, secretsFilePathInPod)
	return dp
}

func addESSAuthSecretsToDeploymentObject(deployment appsv1.Deployment, secretsName string) appsv1.Deployment {
	dp := addSecretsToDeploymentObject(deployment, secretsName, cutil.MMS_AUTH_VOLUME_NAME, cutil.MMS_AUTH_MOUNT_PATH)
	return dp
}

func addESSCertSecretsToDeploymentObject(deployment appsv1.Deployment, secretsName string) appsv1.Deployment {
	dp := addSecretsToDeploymentObject(deployment, secretsName, cutil.MMS_CERT_VOLUME_NAME, cutil.MMS_CERT_MOUNT_PATH)
	return dp
}

// add a reference to the secrets service secrets to the deployment
func addSecretsToDeploymentObject(deployment appsv1.Deployment, secretsName string, volumeName string, volumeMountPath string) appsv1.Deployment {
	// Add secrets (secretsName is $HZN_SERVICE_SECRETS-$agId: hzn-service-secrets-12345) as Volume in deployment
	//volumeName := cutil.SECRETS_VOLUME_NAME
	volume := corev1.Volume{Name: volumeName, VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: secretsName}}}
	volumes := append(deployment.Spec.Template.Spec.Volumes, volume)
	deployment.Spec.Template.Spec.Volumes = volumes

	// mount the volume to deployment containers
	//volumeMountPath := config.HZN_SECRETS_MOUNT
	volumeMount := corev1.VolumeMount{Name: volumeName, MountPath: volumeMountPath}

	// Add secrets as volume mount for containers
	i := len(deployment.Spec.Template.Spec.Containers) - 1
	for i >= 0 {
		newVM := append(deployment.Spec.Template.Spec.Containers[i].VolumeMounts, volumeMount)
		deployment.Spec.Template.Spec.Containers[i].VolumeMounts = newVM
		i--
	}
	return deployment
}
