/*
Copyright 2018 The Kubernetes Authors.

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

package cloudstack

import (
	"encoding/json"
	"fmt"

	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider"
	"k8s.io/autoscaler/cluster-autoscaler/utils/errors"

	apiv1 "k8s.io/api/core/v1"
	klog "k8s.io/klog/v2"
	schedulerframework "k8s.io/kubernetes/pkg/scheduler/framework/v1alpha1"
)

// cluster implements NodeGroup interface.
type cluster struct {
	UUID        string `json:"id"`
	Name        string `json:"name"`
	Minsize     int    `json:"minsize"`
	Maxsize     int    `json:"maxsize"`
	WorkerCount int    `json:"size"`
	MasterCount int    `json:"masternodes"`
	Currentsize int
	VMs         []*node `json:"virtualmachines"`
	manager     *cloudStackManager
}

type node struct {
	Id    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

// MaxSize returns maximum size of the node group.
func (cluster *cluster) MaxSize() int {
	return cluster.Maxsize
}

// MinSize returns minimum size of the node group.
func (cluster *cluster) MinSize() int {
	return cluster.Minsize
}

// TargetSize returns the current TARGET size of the node group. It is possible that the
// number is different from the number of nodes registered in Kubernetes.
func (cluster *cluster) TargetSize() (int, error) {
	return cluster.Currentsize, nil
}

func (cluster *cluster) scaleCluster(delta int) (int, error) {
	return cluster.manager.scaleCluster(delta)
}

// IncreaseSize increases cluster size
func (cluster *cluster) IncreaseSize(delta int) error {

	fmt.Println("IncreaseSize : ", delta)

	klog.Infof("increase cluster:%s with %d nodes", cluster.UUID, delta)
	if delta <= 0 {
		return fmt.Errorf("size increase must be positive")
	}
	if cluster.Currentsize+delta > cluster.Maxsize {
		return fmt.Errorf("size increase is too large - desired : %d max : %d", cluster.Currentsize+delta, cluster.Maxsize)
	}

	// TODO : Return the new cluster and copy everything!
	size, err := cluster.scaleCluster(delta)
	if err != nil {
		return err
	}

	cluster.Currentsize = size
	return nil
}

// DecreaseTargetSize decreases the target size of the node group. This function
// doesn't permit to delete any existing node and can be used only to reduce the
// request for new nodes that have not been yet fulfilled. Delta should be negative.
// It is assumed that cloud provider will not delete the existing nodes if the size
// when there is an option to just decrease the target.
func (cluster *cluster) DecreaseTargetSize(delta int) error {
	return errors.NewAutoscalerError(errors.CloudProviderError, "CloudProvider does not support DecreaseTargetSize")
}

// Belongs returns true if the given node belongs to the NodeGroup.
func (cluster *cluster) Belongs(node *apiv1.Node) (bool, error) {
	// fmt.Println("Belongs : ")
	for _, n := range cluster.VMs {
		if n.Id == node.Status.NodeInfo.SystemUUID { // || n.Id == node.Name || n.Name == node.Name {
			return true, nil
		}
	}
	return false, fmt.Errorf("Unable to find node %s in cluster", node.Name)
}

// DeleteNodes deletes the nodes from the group.
func (cluster *cluster) DeleteNodes(nodes []*apiv1.Node) error {
	b, _ := json.Marshal(nodes)
	fmt.Println("DeleteNodes : ", string(b))

	if cluster.Currentsize <= cluster.Minsize {
		return fmt.Errorf("min size reached, nodes will not be deleted")
	}
	size, err := cluster.manager.deleteNodes(nodes)
	if err != nil {
		return err
	}
	cluster.Currentsize = size
	return nil
}

// Id returns cluster id.
func (cluster *cluster) Id() string {
	return cluster.UUID
}

// Debug returns cluster id.
func (cluster *cluster) Debug() string {
	return fmt.Sprintf("Debug : %s (%d:%d)", cluster.Id(), cluster.MinSize(), cluster.MaxSize())
}

// TemplateNodeInfo returns a node template for this node group.
func (cluster *cluster) TemplateNodeInfo() (*schedulerframework.NodeInfo, error) {
	return nil, cloudprovider.ErrNotImplemented
}

// Exist checks if the node group really exists on the cloud provider side. Allows to tell the
// theoretical node group from the real one.
func (cluster *cluster) Exist() bool {
	return true
}

// Create creates the node group on the cloud provider side.
func (cluster *cluster) Create() (cloudprovider.NodeGroup, error) {
	return nil, cloudprovider.ErrNotImplemented
}

// Autoprovisioned returns true if the node group is autoprovisioned.
func (cluster *cluster) Autoprovisioned() bool {
	return false
}

// Delete deletes the node group on the cloud provider side.
// This will be executed only for autoprovisioned node groups, once their size drops to 0.
func (cluster *cluster) Delete() error {
	return cloudprovider.ErrNotImplemented
}

// Nodes returns a list of all nodes that belong to this node group.
func (cluster *cluster) Nodes() ([]cloudprovider.Instance, error) {
	instances := make([]cloudprovider.Instance, cluster.Currentsize)
	for i := 0; i < cluster.Currentsize; i++ {
		instances[i] = cloudprovider.Instance{
			Id: cluster.VMs[i].Id,
			// Status: cluster.VMs[i].State,
		}
	}
	fmt.Println("Returning Nodes : ", instances)
	return instances, nil
}
