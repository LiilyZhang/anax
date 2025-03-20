package helm3

import (
	"fmt"
	"github.com/boltdb/bolt"
	"github.com/golang/glog"
	"github.com/open-horizon/anax/config"
	"github.com/open-horizon/anax/cutil"
	"github.com/open-horizon/anax/events"
	"github.com/open-horizon/anax/persistence"
	"github.com/open-horizon/anax/resource"
	"github.com/open-horizon/anax/worker"
	//"helm.sh/helm/v3/pkg/cli"
	"path"
)

type Helm3Worker struct {
	worker.BaseWorker
	config    *config.HorizonConfig
	db        *bolt.DB
	authMgr   *resource.AuthenticationManager
	secretMgr *resource.SecretsManager
}

func NewHelm3Worker(name string, config *config.HorizonConfig, db *bolt.DB, am *resource.AuthenticationManager, sm *resource.SecretsManager) *Helm3Worker {
	worker := &Helm3Worker{
		BaseWorker: worker.NewBaseWorker(name, config, nil),
		config:     config,
		db:         db,
		authMgr:    am,
		secretMgr:  sm,
	}
	glog.Info(h3wlog(fmt.Sprintf("Starting Helm3 Worker")))
	worker.Start(worker, 0)
	return worker
}

func (w *Helm3Worker) Messages() chan events.Message {
	return w.BaseWorker.Manager.Messages
}

func (kw *Helm3Worker) GetAuthenticationManager() *resource.AuthenticationManager {
	return kw.authMgr
}

func (w *Helm3Worker) GetSecretManager() *resource.SecretsManager {
	return w.secretMgr
}

func (w *Helm3Worker) NewEvent(incoming events.Message) {
	switch incoming.(type) {
	case *events.AgreementReachedMessage:
		msg, _ := incoming.(*events.AgreementReachedMessage)

		fCmd := NewInstallCommand(msg.LaunchContext())
		w.Commands <- fCmd
	case *events.GovernanceWorkloadCancelationMessage:
		msg, _ := incoming.(*events.GovernanceWorkloadCancelationMessage)

		switch msg.Event().Id {
		case events.AGREEMENT_ENDED:
			cmd := NewUnInstallCommand(msg.AgreementProtocol, msg.AgreementId, msg.ClusterNamespace, msg.Deployment)
			w.Commands <- cmd
		}

	case *events.GovernanceMaintenanceMessage:
		msg, _ := incoming.(*events.GovernanceMaintenanceMessage)

		switch msg.Event().Id {
		case events.CONTAINER_MAINTAIN:
			cmd := NewMaintenanceCommand(msg.AgreementProtocol, msg.AgreementId, msg.ClusterNamespace, msg.Deployment)
			w.Commands <- cmd
		}

	case *events.WorkloadUpdateMessage:
		msg, _ := incoming.(*events.WorkloadUpdateMessage)

		switch msg.Event().Id {
		case events.UPDATE_SECRETS_IN_AGREEMENT:
			cmd := NewUpdateSecretCommand(msg.AgreementProtocol, msg.AgreementId, msg.ClusterNamespaceInAgreement, msg.Deployment, msg.SecretsUpdate)
			w.Commands <- cmd
		}

	case *events.NodeShutdownCompleteMessage:
		msg, _ := incoming.(*events.NodeShutdownCompleteMessage)
		switch msg.Event().Id {
		case events.UNCONFIGURE_COMPLETE:
			w.Commands <- worker.NewTerminateCommand("shutdown")
		}

	default: //nothing

	}
	return

}

func (w *Helm3Worker) CommandHandler(command worker.Command) bool {
	switch command.(type) {
	case *InstallCommand:
		cmd := command.(*InstallCommand)
		if lc := w.getLaunchContext(cmd.LaunchContext); lc == nil {
			glog.Errorf(h3wlog(fmt.Sprintf("incoming event was not a known launch context %T", cmd.LaunchContext)))
		} else {
			glog.V(5).Infof(h3wlog(fmt.Sprintf("LaunchContext(%T) for agreement: %v", lc, lc.AgreementId)))

			// // ignore the native deployment
			if lc.ContainerConfig().Deployment != "" {
				glog.V(5).Infof(h3wlog(fmt.Sprintf("ignoring non-helm3 deployment.")))
				return true
			}

			// Save service secrets from agreement into the microservice instance
			if err := w.GetSecretManager().ProcessServiceSecretsWithInstanceId(lc.AgreementId, lc.AgreementId); err != nil {
				glog.Errorf(h3wlog(fmt.Sprintf("received error saving secrets from agreement into microservice in database, %v", err)))
				return true
			}

			// Check the deployment to check if it is a helm3 deployment
			deploymentConfig := lc.ContainerConfig().ClusterDeployment
			if hd, err := persistence.GetHelm3Deployment(deploymentConfig); err != nil {
				glog.Errorf(h3wlog(fmt.Sprintf("error getting helm3 deployment configuration: %v", err)))
				return true
			} else if _, err := persistence.AgreementDeploymentStarted(w.db, lc.AgreementId, lc.AgreementProtocol, hd); err != nil {
				glog.Errorf(h3wlog(fmt.Sprintf("received error updating database deployment state, %v", err)))
				w.Messages() <- events.NewWorkloadMessage(events.EXECUTION_FAILED, lc.AgreementProtocol, lc.AgreementId, hd)
				return true
			} else if err := w.processHelm3Package(lc, hd); err != nil {
				glog.Errorf(h3wlog(fmt.Sprintf("failed to process helm3 package after agreement negotiation: %v", err)))
				w.Messages() <- events.NewWorkloadMessage(events.EXECUTION_FAILED, lc.AgreementProtocol, lc.AgreementId, hd)
				return true
			} else {
				w.Messages() <- events.NewWorkloadMessage(events.EXECUTION_BEGUN, lc.AgreementProtocol, lc.AgreementId, hd)
			}
		}
	case *UnInstallCommand:
		cmd := command.(*UnInstallCommand)
		glog.V(3).Infof(h3wlog(fmt.Sprintf("uninstalling helm3 release from agreement %v", cmd.CurrentAgreementId)))

		hdc, ok := cmd.Deployment.(*persistence.Helm3DeploymentConfig)
		if !ok {
			glog.Warningf(h3wlog(fmt.Sprintf("ignoring non-Helm3 cancelation command %v", cmd)))
			return true
		} else if err := w.uninstallHelm3Chart(hdc, cmd.CurrentAgreementId, cmd.ClusterNamespace); err != nil {
			glog.Errorf(h3wlog(fmt.Sprintf("failed to uninstall helm3 release %v", cmd.Deployment)))
		}

		w.Messages() <- events.NewWorkloadMessage(events.WORKLOAD_DESTROYED, cmd.AgreementProtocol, cmd.CurrentAgreementId, hdc)
	case *MaintenanceCommand:
		cmd := command.(*MaintenanceCommand)
		glog.V(3).Infof(h3wlog(fmt.Sprintf("received maintenance command %v", cmd)))

		hdc, ok := cmd.Deployment.(*persistence.Helm3DeploymentConfig)
		if !ok {
			glog.Warningf(h3wlog(fmt.Sprintf("ignoring non-Helm3 maintenence command: %v", cmd)))
		} else if err := w.releaseStatus(hdc, "deployed", cmd.AgreementId, cmd.AgreementProtocol, cmd.ClusterNamespace); err != nil {
			glog.Errorf(h3wlog(fmt.Sprintf("%v", err)))
			w.Messages() <- events.NewWorkloadMessage(events.EXECUTION_FAILED, cmd.AgreementProtocol, cmd.AgreementId, hdc)
		}
	case *UpdateSecretCommand:
		cmd := command.(*UpdateSecretCommand)
		glog.V(3).Infof(h3wlog(fmt.Sprintf("receive secret update for agreement: %v", cmd.AgreementId)))

		hdc, ok := cmd.Deployment.(*persistence.Helm3DeploymentConfig)
		if !ok {
			glog.Warningf(h3wlog(fmt.Sprintf("ignoring non-Helm3 secret update command: %v", cmd)))
		} else if err := w.updateHelm3ServiceSecrets(hdc, cmd.AgreementId, cmd.ClusterNamespace, cmd.UpdatedSecrets); err != nil {
			glog.Errorf(h3wlog(fmt.Sprintf("%v", err)))
			w.Messages() <- events.NewWorkloadMessage(events.EXECUTION_FAILED, cmd.AgreementProtocol, cmd.AgreementId, hdc)
		}
	default:
		return true
	}

	return true
}

func (w *Helm3Worker) processHelm3Package(lc *events.AgreementLaunchContext, hd *persistence.Helm3DeploymentConfig) error {
	glog.V(3).Infof(h3wlog(fmt.Sprintf("begin install of Helm3 Deployment release %v for agreement %s", hd.ReleaseName, lc.AgreementId)))

	glog.V(3).Infof(h3wlog(fmt.Sprintf("save service secrets into microservice in the agent database from agreement %s", lc.AgreementId)))
	secretsMap, err := w.GetSecretManager().ProcessServiceSecretsWithInstanceIdForCluster(lc.AgreementId, lc.AgreementId)
	if err != nil {
		return err
	}
	// eg: secretsMap is map[secret1:eyJrZXki...]

	// create auth in agent pod and mount it to service pod
	if ags, err := persistence.FindEstablishedAgreements(w.db, lc.AgreementProtocol, []persistence.EAFilter{persistence.UnarchivedEAFilter(), persistence.IdEAFilter(lc.AgreementId)}); err != nil {
		glog.Errorf("Unable to retrieve agreement %v from database, error %v", lc.AgreementId, err)
	} else if len(ags) != 1 {
		glog.V(3).Infof(h3wlog(fmt.Sprintf("Ignoring the configure event for agreement %v, the agreement is no longer active.", lc.AgreementId)))
		return nil
	} else if ags[0].AgreementTerminatedTime != 0 {
		glog.V(3).Infof(h3wlog(fmt.Sprintf("Received configure command for agreement %v. Ignoring it because this agreement has been terminated.", lc.AgreementId)))
		return nil
	} else if ags[0].AgreementExecutionStartTime != 0 {
		glog.V(3).Infof(h3wlog(fmt.Sprintf("Received configure command for agreement %v. Ignoring it because the containers for this agreement has been configured.", lc.AgreementId)))
		return nil
	} else {
		serviceIdentity := cutil.FormOrgSpecUrl(cutil.NormalizeURL(ags[0].RunningWorkload.URL), ags[0].RunningWorkload.Org)
		sVer := ags[0].RunningWorkload.Version
		glog.V(3).Infof(h3wlog(fmt.Sprintf("Creating ESS creds for svc: %v svcVer: %v", serviceIdentity, sVer)))

		_, err := w.GetAuthenticationManager().CreateCredential(lc.AgreementId, serviceIdentity, sVer, false)
		if err != nil {
			return err
		}

		client, err := NewHelm3Client()
		if err != nil {
			return err
		}

		fssAuthFilePath := path.Join(w.GetAuthenticationManager().GetCredentialPath(lc.AgreementId), config.HZN_FSS_AUTH_FILE) // /var/horizon/ess-auth/<agreementId>/auth.json
		fssCertFilePath := path.Join(w.config.GetESSSSLClientCertPath(), config.HZN_FSS_CERT_FILE)                             // /var/horizon/ess-auth/SSL/cert/cert.pem
		err = client.InstallChart(hd.ChartArchive, hd.ReleaseName, lc.Configure.ClusterNamespace, *(lc.EnvironmentAdditions), fssAuthFilePath, fssCertFilePath, secretsMap, lc.AgreementId)
		if err != nil {
			return err
		}
	}
	return nil

}

func (w *Helm3Worker) getLaunchContext(launchContext interface{}) *events.AgreementLaunchContext {
	switch launchContext.(type) {
	case *events.AgreementLaunchContext:
		lc := launchContext.(*events.AgreementLaunchContext)
		return lc
	}
	return nil
}

func (w *Helm3Worker) uninstallHelm3Chart(hdc *persistence.Helm3DeploymentConfig, agId string, namespace string) error {
	glog.V(3).Infof(h3wlog(fmt.Sprintf("begin uninstall of Helm3 Deployment release %s for agreement %v", hdc.ReleaseName, agId)))

	client, err := NewHelm3Client()
	if err != nil {
		return err
	}
	client.UninstallChart(hdc.ReleaseName, namespace, agId)

	return nil
}

func (w *Helm3Worker) releaseStatus(hdc *persistence.Helm3DeploymentConfig, intendedState string, agId string, agp string, namespace string) error {
	glog.V(3).Infof(h3wlog(fmt.Sprintf("begin listing Helm3 Deployment release %v for agreement %v", hdc.ReleaseName, agId)))

	client, err := NewHelm3Client()
	if err != nil {
		return err
	}
	release, err := client.ReleaseStatus(hdc.ReleaseName, namespace)
	if err != nil {
		return err
	}

	retErrorStr := ""

	if release.Info == nil {
		retErrorStr = fmt.Sprintf("%s %s", retErrorStr, fmt.Sprintf("Release %s info object is nil.", hdc.ReleaseName))
	} else if string(release.Info.Status) != intendedState {
		retErrorStr = fmt.Sprintf("%s %s", retErrorStr, fmt.Sprintf("Release %s has status %s.", hdc.ReleaseName, string(release.Info.Status)))
	}

	if retErrorStr != "" {
		return fmt.Errorf(retErrorStr)
	}

	return nil
}

// updateHelm3ServiceSecrets will update the k8s secret content with the updated secrets value
func (w *Helm3Worker) updateHelm3ServiceSecrets(hdc *persistence.Helm3DeploymentConfig, agId string, namespace string, updatedSecrets []persistence.PersistedServiceSecret) error {
	client, err := NewHelm3Client()
	if err != nil {
		return err
	}
	return client.UpdateServiceSecret(hdc.ReleaseName, agId, namespace, updatedSecrets)
}

var h3wlog = func(v interface{}) string {
	return fmt.Sprintf("Helm3 Worker: %v", v)
}
