package cli

import (
	"github.com/replicatedhq/troubleshoot/pkg/logger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/storageos/kubectl-storageos/pkg/install"
)

func UninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "uninstall",
		Args:         cobra.MinimumNArgs(0),
		Short:        "Uninstall StorageOS",
		Long:         `Uninstall StorageOS and/or ETCD`,
		SilenceUsage: true,
		PreRun: func(cmd *cobra.Command, args []string) {
			viper.BindPFlag(install.StosOperatorNSFlag, cmd.Flags().Lookup(install.StosOperatorNSFlag))
			viper.BindPFlag(install.StosClusterNSFlag, cmd.Flags().Lookup(install.StosClusterNSFlag))
			viper.BindPFlag(install.EtcdNamespaceFlag, cmd.Flags().Lookup(install.EtcdNamespaceFlag))
			viper.BindPFlag(install.StorageOSOnlyFlag, cmd.Flags().Lookup(install.StorageOSOnlyFlag))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			v := viper.GetViper()

			logger.SetQuiet(v.GetBool("quiet"))

			installer, err := install.NewInstaller()
			if err != nil {
				return err
			}

			err = installer.Uninstall()
			if err != nil {
				return err
			}

			return nil
		},
	}
	cmd.Flags().Bool(install.StorageOSOnlyFlag, false, "uninstall storageos only, leaving ETCD untouched")
	cmd.Flags().String(install.EtcdNamespaceFlag, "", "namespace of etcd operator and cluster")
	cmd.Flags().String(install.StosOperatorNSFlag, "", "namespace of storageos operator")
	cmd.Flags().String(install.StosClusterNSFlag, "", "namespace of storageos cluster")

	viper.BindPFlags(cmd.Flags())

	return cmd
}
