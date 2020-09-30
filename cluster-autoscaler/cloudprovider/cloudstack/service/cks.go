package service

import (
	"fmt"
	"strconv"
	"strings"
)

// CKSService provides all the functionality of CloudStack Kubernetes Service
type CKSService interface {
	// Returns the details of the CKS Cluster
	GetClusterDetails(clusterID string) (*Cluster, error)

	// Scales up / down the worker nodes in a cluster
	ScaleCluster(clusterID string, workerCount int) (*Cluster, error)

	// Removes the given nodes from a cluster. However all masters can not be removed
	RemoveNodesFromCluster(clusterID string, nodeIDs []string) (*Cluster, error)
}

// ListClusterResponse is the response returned by the listKubernetesClusters API
type ListClusterResponse struct {
	ClustersResponse *ClustersResponse `json:"listkubernetesclustersresponse"`
}

// ClustersResponse contains the CKS Clusters and their total count
type ClustersResponse struct {
	Count    int        `json:"count"`
	Clusters []*Cluster `json:"kubernetescluster"`
}

// ClusterResponse is the response returned by the scaleKubernetesCluster API
type ClusterResponse struct {
	Cluster *Cluster `json:"kubernetescluster"`
}

// Cluster contains the CKS Cluster details
type Cluster struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Minsize         int               `json:"minsize"`
	Maxsize         int               `json:"maxsize"`
	WorkerCount     int               `json:"size"`
	MasterCount     int               `json:"masternodes"`
	VirtualMachines []*VirtualMachine `json:"virtualmachines"`
}

// VirtualMachine represents a node in a CKS cluster
type VirtualMachine struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
}

type cksService struct {
	client APIClient
}

func (service *cksService) GetClusterDetails(clusterID string) (*Cluster, error) {
	var out ListClusterResponse
	_, err := service.client.NewRequest("listKubernetesClusters", map[string]string{
		"id": clusterID,
	}, &out)

	if err != nil {
		return nil, fmt.Errorf("Unable to fetch cluster details : %v", err)
	}

	clusters := out.ClustersResponse.Clusters
	if len(clusters) == 0 {
		return nil, fmt.Errorf("Unable to fetch cluster with id : %v", clusterID)
	}
	return clusters[0], err
}

func (service *cksService) ScaleCluster(clusterID string, workerCount int) (*Cluster, error) {
	var out ClusterResponse
	_, err := service.client.NewRequest("scaleKubernetesCluster", map[string]string{
		"id":   clusterID,
		"size": strconv.Itoa(workerCount),
	}, &out)

	if err != nil {
		return nil, fmt.Errorf("Unable to scale cluster : %v", err)
	}
	return out.Cluster, err
}

func (service *cksService) RemoveNodesFromCluster(clusterID string, nodeIDs []string) (*Cluster, error) {
	var out ClusterResponse
	_, err := service.client.NewRequest("scaleKubernetesCluster", map[string]string{
		"id":      clusterID,
		"nodeids": strings.Join(nodeIDs[:], ","),
	}, &out)
	if err != nil {
		return nil, fmt.Errorf("Unable to delete %v from cluster : %v", nodeIDs, err)
	}
	return out.Cluster, err
}

// NewCKSService returns a new CKS Service
func NewCKSService(config *Config) CKSService {
	client := NewAPIClient(config)
	return &cksService{
		client: client,
	}
}
