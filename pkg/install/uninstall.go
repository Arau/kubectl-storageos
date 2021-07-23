package install

import (
	"context"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
	pluginutils "github.com/storageos/kubectl-storageos/pkg/utils"
	"sigs.k8s.io/kustomize/api/krusty"
)

const (
	StorageOSOnlyFlag = "storageos-only"
)

// Uninstall performs storageos and etcd uninstallation for kubectl-storageos
func (in *Installer) Uninstall() error {
	v := viper.GetViper()
	err := in.uninstallStorageOS(v.GetString(StosOperatorNSFlag), v.GetString(StosClusterNSFlag))
	if err != nil {
		return err
	}

	// return early if user only wishes to delete storageos, leaving etcd untouched
	if v.GetBool(StorageOSOnlyFlag) {
		return nil
	}
	err = in.uninstallEtcd(v.GetString(EtcdNamespaceFlag))
	if err != nil {
		return err
	}

	return nil
}

func (in *Installer) uninstallStorageOS(stosOperatorNS, stosClusterNS string) error {
	var err error
	// add changes to storageos kustomizations here before kustomizeAndiDelete calls ie make changes
	// to storageos/operator/kustomization.yaml and/or storageos/cluster/kustomization.yaml
	// based on flags (or cli config file)
	if stosOperatorNS != "" {
		err := in.setFieldInFsManifest(filepath.Join(stosDir, operatorDir, kustomizationFile), stosOperatorNS, "namespace", "")
		if err != nil {
			return err
		}

	} else {
		stosOperatorNS = defaultStosOperatorNS
	}

	if stosClusterNS != "" {
		err = in.setFieldInFsManifest(filepath.Join(stosDir, clusterDir, kustomizationFile), stosClusterNS, "namespace", "")
		if err != nil {
			return err
		}
	}

	err = in.kustomizeAndDelete(filepath.Join(stosDir, clusterDir))
	if err != nil {
		return err
	}
	// sleep to allow operator to terminate cluster's child objects
	time.Sleep(5 * time.Second)

	// Remove namespace from multi-doc to avoid complications during deletion
	err = in.omitKindFromFSMultiDoc(filepath.Join(stosDir, operatorDir, stosOperatorFile), "Namespace")
	if err != nil {
		return err
	}
	err = in.kustomizeAndDelete(filepath.Join(stosDir, operatorDir))
	if err != nil {
		return err
	}

	// postpone namespace deletion until last after delay
	// TODO: check here to ensure no objects exist in NS
	time.Sleep(5 * time.Second)

	if stosClusterNS != "" {
		err = in.kubectlClient.Delete(context.TODO(), "", pluginutils.NamespaceYaml(stosClusterNS), true)
		if err != nil {
			return err
		}
	}
	err = in.kubectlClient.Delete(context.TODO(), "", pluginutils.NamespaceYaml(stosOperatorNS), true)
	if err != nil {
		return err
	}
	return nil
}

func (in *Installer) uninstallEtcd(etcdNamespace string) error {
	var err error
	// add changes to etcd kustomizations here before kustomizeAndDelete calls ie make changes
	// to etcd/operator/kustomization.yaml and/or etcd/cluster/kustomization.yaml
	// based on flags (or cli config file)
	if etcdNamespace != "" {
		err = in.setFieldInFsManifest(filepath.Join(etcdDir, operatorDir, kustomizationFile), etcdNamespace, "namespace", "")
		if err != nil {
			return err
		}
		err = in.setFieldInFsManifest(filepath.Join(etcdDir, clusterDir, kustomizationFile), etcdNamespace, "namespace", "")
		if err != nil {
			return err
		}

	} else {
		etcdNamespace = defaultEtcdClusterNS
	}

	err = in.kustomizeAndDelete(filepath.Join(etcdDir, clusterDir))
	if err != nil {
		return err
	}
	// sleep to allow operator to terminate cluster's child objects
	time.Sleep(5 * time.Second)

	// Remove namespace from multi-doc to avoid complications during deletion
	err = in.omitKindFromFSMultiDoc(filepath.Join(etcdDir, operatorDir, etcdOperatorFile), "Namespace")
	if err != nil {
		return err
	}

	err = in.kustomizeAndDelete(filepath.Join(etcdDir, operatorDir))
	if err != nil {
		return err
	}

	// postpone namespace deletion until last after delay
	// TODO: check here to ensure no objects exist in NS
	time.Sleep(5 * time.Second)
	err = in.kubectlClient.Delete(context.TODO(), "", pluginutils.NamespaceYaml(etcdNamespace), true)
	if err != nil {
		return err
	}

	return nil
}

// kustomizeAndDelete performs kustomize run on the provided dir and kubect delete on the files in dir.
// It is the equivalent of:
// `kustomize build <dir> | kubectl delete -f -
func (in *Installer) kustomizeAndDelete(dir string) error {
	kustomizer := krusty.MakeKustomizer(krusty.MakeDefaultOptions())
	resMap, err := kustomizer.Run(in.fileSys, dir)
	if err != nil {
		return err
	}
	resYaml, err := resMap.AsYaml()
	if err != nil {
		return err
	}
	err = in.kubectlClient.Delete(context.TODO(), "", string(resYaml), true)
	if err != nil {
		return err
	}

	return nil
}

// omitKindFromFSMultiDoc reads the file at path of the in-memory filesystem, uses OmitKindFromMultiDoc
// internally to perform the update and then writes the returned file to path.
func (in *Installer) omitKindFromFSMultiDoc(path, kind string) error {
	data, err := in.fileSys.ReadFile(path)
	if err != nil {
		return err
	}
	dataStr, err := pluginutils.OmitKindFromMultiDoc(string(data), kind)
	if err != nil {
		return err
	}
	err = in.fileSys.WriteFile(path, []byte(dataStr))
	if err != nil {
		return err
	}
	return nil
}
