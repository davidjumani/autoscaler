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

	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/autoscaler/cluster-autoscaler/config"
)

type acsConfig struct {
	APIKey    string
	SecretKey string
	Endpoint  string
}

type cloudStackManager struct {
	clusterID      string
	cluster        *cluster
	config         *acsConfig
	client         cloudStackClient
	mux            sync.Mutex
	maxClusterSize int
	minClusterSize int
}

func (manager *cloudStackManager) clusterForNode(node *v1.Node) (*cluster, error) {
	_, err := manager.cluster.Belongs(node)
	if err != nil {
		return nil, err
	}
	return manager.cluster, nil
}

func (manager *cloudStackManager) refresh() error {
	return manager.fetchCluster()
}

func (manager *cloudStackManager) cleanup() error {
	manager.mux.Lock()
	defer manager.mux.Unlock()
	return nil
}

func (manager *cloudStackManager) fetchCluster() error {
	manager.mux.Lock()
	defer manager.mux.Unlock()

	var response listClusterResponse
	_, err := manager.client.NewAPIRequest("listKubernetesClusters", map[string]string{
		"id": manager.clusterID,
	}, false, &response)
	if err != nil {
		return fmt.Errorf("Unable to fetch cluster details : %v", err)
	}

	clusters := response.ClustersResponse.Clusters
	if len(clusters) == 0 {
		return fmt.Errorf("Unable to fetch cluster with id : %v", manager.clusterID)
	}

	cluster := clusters[0]
	fmt.Println("Got cluster : ", cluster)
	cluster.Currentsize = cluster.MasterCount + cluster.WorkerCount
	cluster.Maxsize = manager.maxClusterSize
	cluster.Minsize = manager.minClusterSize
	cluster.manager = manager

	manager.cluster = cluster
	return nil
}

func (manager *cloudStackManager) scaleCluster(delta int) (int, error) {
	manager.mux.Lock()
	defer manager.mux.Unlock()

	var out clusterResponse
	_, err := manager.client.NewAPIRequest("scaleKubernetesCluster", map[string]string{
		"id":   manager.clusterID,
		"size": strconv.Itoa(manager.cluster.Currentsize + delta),
	}, true, &out)
	if err != nil {
		err = fmt.Errorf("Unable to scale cluster : %v", err)
		return 0, err
	}
	fmt.Println("Scaled up cluster : ", out.Cluster)
	cluster := out.Cluster
	cluster.Currentsize = cluster.MasterCount + cluster.WorkerCount
	cluster.Maxsize = manager.maxClusterSize
	cluster.Minsize = manager.minClusterSize
	cluster.manager = manager

	manager.cluster = cluster
	return len(manager.cluster.VMs), nil
}

func (manager *cloudStackManager) deleteNodes(nodes []*apiv1.Node) (int, error) {
	manager.mux.Lock()
	defer manager.mux.Unlock()

	nodeIDs := make([]string, len(nodes))
	for _, node := range nodes {
		if !strings.Contains(node.Name, "-master-") {
			nodeIDs = append(nodeIDs, node.Status.NodeInfo.SystemUUID)
		} else {
			fmt.Println("Trying to remove the master node!!!!!!")
		}
	}

	var out clusterResponse
	_, err := manager.client.NewAPIRequest("scaleKubernetesCluster", map[string]string{
		"nodeids": strings.Join(nodeIDs[:], ","),
	}, true, &out)
	if err != nil {
		err = fmt.Errorf("Unable to delete %v from cluster : %v", nodeIDs, err)
		return 0, err
	}
	fmt.Println("Scaled down cluster : ", out.Cluster)

	cluster := out.Cluster
	cluster.Currentsize = cluster.MasterCount + cluster.WorkerCount
	cluster.Maxsize = manager.maxClusterSize
	cluster.Minsize = manager.minClusterSize
	cluster.manager = manager

	manager.cluster = cluster
	return len(manager.cluster.VMs), nil
}

func newManager(opts config.AutoscalingOptions) (*cloudStackManager, error) {
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

	// clusterID, ok := os.LookupEnv("CLUSTER_ID")
	// if !ok {
	// 	return nil, fmt.Errorf("CLUSTER_ID environment variable not set")
	// }

	if len(opts.NodeGroups) == 0 {
		return nil, fmt.Errorf("Cluster details not present. Please use the --nodes=<min>:<max>:<id>")
	}

	clusterDetails := strings.Split(opts.NodeGroups[0], ":")
	if len(clusterDetails) != 3 {
		return nil, fmt.Errorf("Cluster details not present. Please use the --nodes=<min>:<max>:<id>")
	}

	config := &acsConfig{
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

	manager := &cloudStackManager{
		config:         config,
		client:         newClient(config),
		minClusterSize: minClusterSize,
		maxClusterSize: maxClusterSize,
		clusterID:      clusterDetails[2],
	}
	manager.refresh()
	return manager, nil
}
