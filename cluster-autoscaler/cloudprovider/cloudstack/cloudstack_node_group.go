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
	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider/cloudstack/service"
	"k8s.io/autoscaler/cluster-autoscaler/utils/errors"

	apiv1 "k8s.io/api/core/v1"
	klog "k8s.io/klog/v2"
	schedulerframework "k8s.io/kubernetes/pkg/scheduler/framework/v1alpha1"
)

// asg implements NodeGroup interface.
type asg struct {
	uuid            string
	name            string
	minSize         int
	maxSize         int
	masterNodesSize int
	workerNodesSize int
	currentSize     int
	instances       []cloudprovider.Instance
	manager         *manager
}

func (asg *asg) Copy(cluster *service.Cluster, manager *manager) {
	asg.uuid = cluster.ID
	asg.name = cluster.Name
	asg.minSize = cluster.Minsize
	asg.maxSize = cluster.Maxsize
	asg.masterNodesSize = cluster.MasterCount
	asg.workerNodesSize = cluster.WorkerCount
	asg.currentSize = cluster.WorkerCount + cluster.MasterCount
	asg.manager = manager
	instances := make([]cloudprovider.Instance, len(cluster.VirtualMachines))
	for i := 0; i < len(cluster.VirtualMachines); i++ {
		instances[i] = cloudprovider.Instance{
			Id: cluster.VirtualMachines[i].ID,
			// Status: cluster.VMs[i].State,
		}
	}
	asg.instances = instances
}

// MaxSize returns maximum size of the node group.
func (asg *asg) MaxSize() int {
	return asg.maxSize
}

// MinSize returns minimum size of the node group.
func (asg *asg) MinSize() int {
	return asg.minSize
}

// TargetSize returns the current TARGET size of the node group. It is possible that the
// number is different from the number of nodes registered in Kubernetes.
func (asg *asg) TargetSize() (int, error) {
	return asg.currentSize, nil
}

// IncreaseSize increases cluster size
func (asg *asg) IncreaseSize(delta int) error {

	fmt.Println("IncreaseSize : ", delta)

	klog.Infof("Increase Cluster :%s by %d", asg.uuid, delta)
	if delta <= 0 {
		return fmt.Errorf("Delta must be positive")
	}
	if asg.currentSize+delta > asg.maxSize {
		return fmt.Errorf("Delta too large - Wanted : %d Max : %d", asg.currentSize+delta, asg.maxSize)
	}

	_, err := asg.manager.scaleCluster(asg.workerNodesSize + delta)
	return err
}

// DecreaseTargetSize decreases the target size of the node group. This function
// doesn't permit to delete any existing node and can be used only to reduce the
// request for new nodes that have not been yet fulfilled. Delta should be negative.
// It is assumed that cloud provider will not delete the existing nodes if the size
// when there is an option to just decrease the target.
func (asg *asg) DecreaseTargetSize(delta int) error {
	return errors.NewAutoscalerError(errors.CloudProviderError, "CloudProvider does not support DecreaseTargetSize")
}

// Belongs returns true if the given node belongs to the NodeGroup.
func (asg *asg) Belongs(node *apiv1.Node) (bool, error) {
	// fmt.Println("Belongs : ")
	for _, instance := range asg.instances {
		if instance.Id == node.Status.NodeInfo.SystemUUID { // || n.Id == node.Name || n.Name == node.Name {
			return true, nil
		}
	}
	return false, fmt.Errorf("Unable to find node %s in cluster", node.Name)
}

// DeleteNodes deletes the nodes from the group.
func (asg *asg) DeleteNodes(nodes []*apiv1.Node) error {
	b, _ := json.Marshal(nodes)
	fmt.Println("DeleteNodes : ", string(b))

	if asg.currentSize <= asg.minSize {
		return fmt.Errorf("Min size reached. Can not delete %v nodes", len(nodes))
	}
	_, err := asg.manager.deleteNodes(nodes)
	return err
}

// Id returns cluster id.
func (asg *asg) Id() string {
	return asg.uuid
}

// Debug returns cluster id.
func (asg *asg) Debug() string {
	return fmt.Sprintf("Debug : %s [%d : %d]", asg.uuid, asg.minSize, asg.maxSize)
}

// TemplateNodeInfo returns a node template for this node group.
func (asg *asg) TemplateNodeInfo() (*schedulerframework.NodeInfo, error) {
	return nil, cloudprovider.ErrNotImplemented
}

// Exist checks if the node group really exists on the cloud provider side. Allows to tell the
// theoretical node group from the real one.
func (asg *asg) Exist() bool {
	return true
}

// Create creates the node group on the cloud provider side.
func (asg *asg) Create() (cloudprovider.NodeGroup, error) {
	return nil, cloudprovider.ErrNotImplemented
}

// Autoprovisioned returns true if the node group is autoprovisioned.
func (asg *asg) Autoprovisioned() bool {
	return false
}

// Delete deletes the node group on the cloud provider side.
// This will be executed only for autoprovisioned node groups, once their size drops to 0.
func (asg *asg) Delete() error {
	return cloudprovider.ErrNotImplemented
}

// Nodes returns a list of all nodes that belong to this node group.
func (asg *asg) Nodes() ([]cloudprovider.Instance, error) {
	return asg.instances, nil
}
