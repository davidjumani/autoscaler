package cloudstack

type listClusterResponse struct {
	ClustersResponse *clustersResponse `json:"listkubernetesclustersresponse"`
}

type clustersResponse struct {
	Count    int        `json:"count"`
	Clusters []*cluster `json:"kubernetescluster"`
}

type clusterResponse struct {
	Cluster *cluster `json:"kubernetescluster"`
}
