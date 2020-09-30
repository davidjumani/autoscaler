/*
Copyright 2017 The Kubernetes Authors.

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
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"k8s.io/autoscaler/cluster-autoscaler/cloudprovider/cloudstack/service"
	"k8s.io/autoscaler/cluster-autoscaler/config"

	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
)

type acsConfig struct {
	APIKey    string
	SecretKey string
	Endpoint  string
}

type manager struct {
	clusterID      string
	asg            *asg
	service        service.CKSService
	mux            sync.Mutex
	maxClusterSize int
	minClusterSize int
}

func (manager *manager) clusterForNode(node *v1.Node) (*asg, error) {
	_, err := manager.asg.Belongs(node)
	if err != nil {
		return nil, err
	}
	return manager.asg, nil
}

func (manager *manager) refresh() error {
	return manager.fetchCluster()
}

func (manager *manager) cleanup() error {
	manager.service.Close()
	manager.mux.Lock()
	defer manager.mux.Unlock()
	return nil
}

// func (manager *manager) setUp

func (manager *manager) fetchCluster() error {
	manager.mux.Lock()
	defer manager.mux.Unlock()

	// TODO : Maybe instead hit an api to say that autoscaling is enabled ?
	cluster, err := manager.service.GetClusterDetails(manager.clusterID)
	if err != nil {
		return err
	}

	fmt.Println("Got cluster : ", cluster)
	// TODO : Maybe move this all to a separate function ?
	manager.asg.Copy(cluster, manager)
	manager.asg.maxSize = manager.maxClusterSize
	manager.asg.minSize = manager.minClusterSize

	return nil
}

func (manager *manager) scaleCluster(workerCount int) (int, error) {
	manager.mux.Lock()
	defer manager.mux.Unlock()

	cluster, err := manager.service.ScaleCluster(manager.clusterID, workerCount)
	if err != nil {
		return 0, err
	}
	fmt.Println("Scaled up cluster : ", cluster)
	manager.asg.Copy(cluster, manager)
	manager.asg.maxSize = manager.maxClusterSize
	manager.asg.minSize = manager.minClusterSize

	return len(manager.asg.instances), nil
}

func (manager *manager) deleteNodes(nodes []*apiv1.Node) (int, error) {
	manager.mux.Lock()
	defer manager.mux.Unlock()

	nodeIDs := make([]string, len(nodes))
	for _, node := range nodes {
		nodeIDs = append(nodeIDs, node.Status.NodeInfo.SystemUUID)
	}

	if len(nodeIDs) == 0 {
		return 0, fmt.Errorf("Unable to fetch nodeids from %v", nodes)
	}

	cluster, err := manager.service.RemoveNodesFromCluster(manager.clusterID, nodeIDs)
	if err != nil {
		return 0, err
	}
	fmt.Println("Scaled down cluster : ", cluster)

	manager.asg.Copy(cluster, manager)
	manager.asg.maxSize = manager.maxClusterSize
	manager.asg.minSize = manager.minClusterSize

	return len(manager.asg.instances), nil
}

func newManager(opts config.AutoscalingOptions) (*manager, error) {
	apiKey, ok := os.LookupEnv("API_KEY")
	if !ok {
		return nil, fmt.Errorf("API_KEY environment variable not set")
	}

	secretKey, ok := os.LookupEnv("SECRET_KEY")
	if !ok {
		return nil, fmt.Errorf("SECRET_KEY environment variable not set")
	}

	endpoint, ok := os.LookupEnv("ENDPOINT")
	if !ok {
		return nil, fmt.Errorf("ENDPOINT environment variable not set")
	}

	if len(opts.NodeGroups) == 0 {
		return nil, fmt.Errorf("Cluster details not present. Please use the --nodes=<min>:<max>:<id>")
	}

	clusterDetails := strings.Split(opts.NodeGroups[0], ":")
	if len(clusterDetails) != 3 {
		return nil, fmt.Errorf("Cluster details not present. Please use the --nodes=<min>:<max>:<id>")
	}

	config := &service.Config{
		APIKey:    apiKey,
		SecretKey: secretKey,
		Endpoint:  endpoint,
	}

	minClusterSize, err := strconv.Atoi(clusterDetails[0])
	if err != nil {
		return nil, fmt.Errorf("Invalid value for min cluster size %s : %v", clusterDetails[0], err)
	}

	maxClusterSize, err := strconv.Atoi(clusterDetails[1])
	if err != nil {
		return nil, fmt.Errorf("Invalid value for max cluster size %s : %v", clusterDetails[1], err)
	}

	manager := &manager{
		service:        service.NewCKSService(config),
		minClusterSize: minClusterSize,
		maxClusterSize: maxClusterSize,
		clusterID:      clusterDetails[2],
		asg:            &asg{},
	}
	manager.refresh()
	return manager, nil
}
