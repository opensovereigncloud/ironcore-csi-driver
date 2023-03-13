// Copyright 2023 OnMetal authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package driver

import (
	"context"
	"net"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/dell/gocsi"
	"github.com/onmetal/onmetal-csi-driver/cmd/options"
	"k8s.io/klog/v2"
	"k8s.io/mount-utils"
	utilexec "k8s.io/utils/exec"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type driver struct {
	mountUtil     *mount.SafeFormatAndMount
	targetClient  client.Client
	onmetalClient client.Client
	config        *options.Config
}

// Driver is the CSI Mock driver provider.
type Driver interface {
	csi.ControllerServer
	csi.IdentityServer
	csi.NodeServer

	BeforeServe(context.Context, *gocsi.StoragePlugin, net.Listener) error
}

func NewDriver(config *options.Config, targetClient, onMetalClient client.Client) Driver {
	klog.InfoS("Driver Information", "Driver", CSIDriverName, "Version", "")
	return &driver{
		config:        config,
		targetClient:  targetClient,
		onmetalClient: onMetalClient,
		mountUtil:     &mount.SafeFormatAndMount{Interface: mount.New(""), Exec: utilexec.New()},
	}
}

func (d *driver) BeforeServe(_ context.Context, _ *gocsi.StoragePlugin, _ net.Listener) error {
	return nil
}
