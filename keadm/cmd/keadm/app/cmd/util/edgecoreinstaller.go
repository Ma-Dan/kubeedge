/*
Copyright 2019 The KubeEdge Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"fmt"
	"os"
	"strings"

	types "github.com/kubeedge/kubeedge/keadm/cmd/keadm/app/cmd/common"
	"github.com/kubeedge/kubeedge/pkg/apis/componentconfig/edgecore/v1alpha1"
)

// KubeEdgeInstTool embedes Common struct and contains cloud node ip:port information
// It implements ToolsInstaller interface
type KubeEdgeInstTool struct {
	Common
	CertPath              string
	CloudCoreIP           string
	EdgeNodeName          string
	EdgeNodeIP            string
	Region                string
	ConfigPath            string
	RuntimeType           string
	RemoteRuntimeEndpoint string
	Token                 string
	CertPort              string
	QuicPort              string
	TunnelPort            string
	CGroupDriver          string
	TarballPath           string
}

// InstallTools downloads KubeEdge for the specified verssion
// and makes the required configuration changes and initiates edgecore.
func (ku *KubeEdgeInstTool) InstallTools() error {
	ku.SetOSInterface(GetOSInterface())

	edgeCoreRunning, err := ku.IsKubeEdgeProcessRunning(KubeEdgeBinaryName)
	if err != nil {
		return err
	}
	if edgeCoreRunning {
		return fmt.Errorf("EdgeCore is already running on this node, please run reset to clean up first")
	}

	ku.SetKubeEdgeVersion(ku.ToolVersion)

	opts := &types.InstallOptions{
		TarballPath:   ku.TarballPath,
		ComponentType: types.EdgeCore,
	}

	if ku.Region == "en" {
		KubeEdgeDownloadURL = "https://github.com/kubeedge/kubeedge/releases/download"
		ServiceFileURLFormat = "https://raw.githubusercontent.com/kubeedge/kubeedge/release-%s/build/tools/%s"
	}

	err = ku.InstallKubeEdge(*opts)
	if err != nil {
		return err
	}

	err = ku.createEdgeConfigFiles()
	if err != nil {
		return err
	}

	err = ku.RunEdgeCore()
	if err != nil {
		return err
	}
	return nil
}

func (ku *KubeEdgeInstTool) createEdgeConfigFiles() error {
	//This makes sure the path is created, if it already exists also it is fine
	err := os.MkdirAll(KubeEdgeConfigDir, os.ModePerm)
	if err != nil {
		return fmt.Errorf("not able to create %s folder path", KubeEdgeConfigDir)
	}

	edgeCoreConfig := v1alpha1.NewDefaultEdgeCoreConfig()
	edgeCoreConfig.Modules.EdgeHub.WebSocket.Server = ku.CloudCoreIP

	if ku.EdgeNodeName != "" {
		edgeCoreConfig.Modules.Edged.HostnameOverride = ku.EdgeNodeName
	}
	if ku.EdgeNodeIP != "" {
		edgeCoreConfig.Modules.Edged.NodeIP = ku.EdgeNodeIP
	}
	if ku.RuntimeType != "" {
		edgeCoreConfig.Modules.Edged.RuntimeType = ku.RuntimeType
	}
	if ku.CGroupDriver != "" {
		switch ku.CGroupDriver {
		case v1alpha1.CGroupDriverSystemd:
			edgeCoreConfig.Modules.Edged.CGroupDriver = v1alpha1.CGroupDriverSystemd
		case v1alpha1.CGroupDriverCGroupFS:
			edgeCoreConfig.Modules.Edged.CGroupDriver = v1alpha1.CGroupDriverCGroupFS
		default:
			return fmt.Errorf("unsupported CGroupDriver: %s", ku.CGroupDriver)
		}
	}

	if ku.RemoteRuntimeEndpoint != "" {
		edgeCoreConfig.Modules.Edged.RemoteRuntimeEndpoint = ku.RemoteRuntimeEndpoint
		edgeCoreConfig.Modules.Edged.RemoteImageEndpoint = ku.RemoteRuntimeEndpoint
	}
	if ku.Token != "" {
		edgeCoreConfig.Modules.EdgeHub.Token = ku.Token
	}
	if ku.CertPort != "" {
		edgeCoreConfig.Modules.EdgeHub.HTTPServer = "https://" + strings.Split(ku.CloudCoreIP, ":")[0] + ":" + ku.CertPort
	} else {
		edgeCoreConfig.Modules.EdgeHub.HTTPServer = "https://" + strings.Split(ku.CloudCoreIP, ":")[0] + ":10002"
	}
	if ku.QuicPort != "" {
		edgeCoreConfig.Modules.EdgeHub.Quic.Server = strings.Split(ku.CloudCoreIP, ":")[0] + ":" + ku.QuicPort
	} else {
		edgeCoreConfig.Modules.EdgeHub.Quic.Server = strings.Split(ku.CloudCoreIP, ":")[0] + ":10001"
	}
	if ku.TunnelPort != "" {
		edgeCoreConfig.Modules.EdgeStream.TunnelServer = strings.Split(ku.CloudCoreIP, ":")[0] + ":" + ku.TunnelPort
	} else {
		edgeCoreConfig.Modules.EdgeStream.TunnelServer = strings.Split(ku.CloudCoreIP, ":")[0] + ":10004"
	}

	edgeCoreConfig.Modules.EdgeStream.Enable = true

	if ku.ToolVersion.Major == 1 && ku.ToolVersion.Minor == 2 {
		edgeCoreConfig.Modules.EdgeHub.TLSPrivateKeyFile = strings.Join([]string{KubeEdgeCloudDefaultCertPath, "server.key"}, "")
		edgeCoreConfig.Modules.EdgeHub.TLSCertFile = strings.Join([]string{KubeEdgeCloudDefaultCertPath, "server.crt"}, "")
	}
	if err := types.Write2File(KubeEdgeEdgeCoreNewYaml, edgeCoreConfig); err != nil {
		return err
	}
	return nil
}

//TearDown method will remove the edge node from api-server and stop edgecore process
func (ku *KubeEdgeInstTool) TearDown() error {
	ku.SetOSInterface(GetOSInterface())
	ku.SetKubeEdgeVersion(ku.ToolVersion)

	//Kill edge core process
	if err := ku.KillKubeEdgeBinary(KubeEdgeBinaryName); err != nil {
		return err
	}

	return nil
}
