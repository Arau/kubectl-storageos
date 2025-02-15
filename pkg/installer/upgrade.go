package installer

import (
	"context"
	"path/filepath"
	"time"

	operatorapi "github.com/storageos/cluster-operator/pkg/apis/storageos/v1"
	apiv1 "github.com/storageos/kubectl-storageos/api/v1"
	pluginutils "github.com/storageos/kubectl-storageos/pkg/utils"
	pluginversion "github.com/storageos/kubectl-storageos/pkg/version"
)

func Upgrade(uninstallConfig *apiv1.KubectlStorageOSConfig, installConfig *apiv1.KubectlStorageOSConfig, versionToUninstall string) error {
	// create new installer with in-mem fs of operator and cluster to be installed
	// use installer to validate etcd-endpoints before going any further
	installer, err := NewInstaller(installConfig, true)
	if err != nil {
		return err
	}
	storageOSCluster, err := pluginutils.GetStorageOSCluster(installer.clientConfig, "")
	if err != nil {
		return err
	}

	// if etcdEndpoints was not passed via config, use that of existing cluster
	if installConfig.Spec.Install.EtcdEndpoints == "" {
		installConfig.Spec.Install.EtcdEndpoints = storageOSCluster.Spec.KVBackend.Address
	}

	err = installer.handleEndpointsInput(installConfig.Spec.Install.EtcdEndpoints)
	if err != nil {
		return err
	}

	// create (un)installer with in-mem fs of operator and cluster to be uninstalled
	uninstaller, err := NewInstaller(uninstallConfig, false)
	if err != nil {
		return err
	}

	err = uninstaller.prepareForUpgrade(installConfig, storageOSCluster, versionToUninstall)
	if err != nil {
		return err
	}

	// uninstall existing storageos operator and cluster
	err = uninstaller.Uninstall(uninstallConfig, true)
	if err != nil {
		return err
	}

	// sleep to allow CRDs to be removed
	// TODO: Add specific check instead of sleep
	time.Sleep(30 * time.Second)

	// install new storageos operator and cluster
	err = installer.installStorageOS(installConfig)
	if err != nil {
		return err
	}

	return nil
}

// prepareForUpgrade performs necessary steps before upgrade commences
func (in *Installer) prepareForUpgrade(installConfig *apiv1.KubectlStorageOSConfig, storageOSCluster *operatorapi.StorageOSCluster, versionToUninstall string) error {
	// write storageoscluster, secret and storageclass manifests to disk
	err := in.writeBackupFileSystem(storageOSCluster)
	if err != nil {
		return err
	}

	// apply the storageclass manifest written to disk (now with finalizer to prevent deletion by operator)
	err = in.applyBackupManifestWithFinalizer(stosStorageClassFile)
	if err != nil {
		return err
	}

	// if the version being uninstalled during upgrade is that of the 'old' operator (pre v2.5) existing
	// CSI secrets are applied with finalizer to prevent deletion by operator
	storageosV1, err := pluginversion.VersionIsLessThanOrEqual(versionToUninstall, pluginversion.ClusterOperatorLastVersion())
	if err != nil {
		return err
	}
	if storageosV1 {
		err = in.applyBackupManifestWithFinalizer(csiSecretsFile)
		if err != nil {
			return err
		}
	}

	// discover uninstalled secret username and password for upgrade. Here we use (1) the (un)installer
	// as it contains the on-disk FS of the uninstalled secrets and (2) the installConfig so we can
	// set secret username and password in the secret manifest to be installed later
	err = in.copyStorageOSSecretData(installConfig)
	if err != nil {
		return err
	}

	return nil
}

// copyStorageOSSecretData uses the (un)installer's on-disk filesystem to read the username and password
// of the storageos secret which is to be uninstalled. This data is then copied to the installConfig so
// that it can be added to the new storageos secret to be created during the install phase of the upgrade
func (in *Installer) copyStorageOSSecretData(installConfig *apiv1.KubectlStorageOSConfig) error {
	backupPath, err := in.getBackupPath()
	if err != nil {
		return err
	}
	stosSecrets, err := in.onDiskFileSys.ReadFile(filepath.Join(backupPath, stosSecretsFile))
	if err != nil {
		return err
	}
	storageosAPISecret, err := pluginutils.GetManifestFromMultiDocByName(string(stosSecrets), "storageos-api")
	if err != nil {
		return err
	}
	secretUsername, err := pluginutils.GetFieldInManifest(storageosAPISecret, "data", "apiUsername")
	if err != nil {
		return err
	}
	secretPassword, err := pluginutils.GetFieldInManifest(storageosAPISecret, "data", "apiPassword")
	if err != nil {
		return err
	}

	installConfig.InstallerMeta.SecretUsername = secretUsername
	installConfig.InstallerMeta.SecretPassword = secretPassword

	return nil
}

// applyBackupManifest applies file from the (un)installer's on-disk filesystem with finalizer
func (in *Installer) applyBackupManifestWithFinalizer(file string) error {
	backupPath, err := in.getBackupPath()
	if err != nil {
		return err
	}

	multidoc, err := in.onDiskFileSys.ReadFile(filepath.Join(backupPath, file))
	if err != nil {
		return err
	}

	manifests := splitMultiDoc(string(multidoc))
	for _, manifest := range manifests {
		// if a finalizer already exists for this object, continue.
		// This may be the case if an upgrade has already occured.
		existingFinalizers, err := pluginutils.GetFieldInManifest(string(manifest), "metadata", "finalizers")
		if err != nil {
			return err
		}
		if existingFinalizers != "" {
			continue
		}

		// add finalizer to manifest (mutated manifest is not written to disk)
		manifestWithFinaliser, err := pluginutils.SetFieldInManifest(string(manifest), "- "+stosFinalizer, "finalizers", "metadata")
		if err != nil {
			return err
		}
		err = in.kubectlClient.Apply(context.TODO(), "", string(manifestWithFinaliser), true)
		if err != nil {
			return err
		}
	}
	return nil
}
