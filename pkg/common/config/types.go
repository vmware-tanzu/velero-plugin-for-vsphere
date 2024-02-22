package config

type Config struct {
	Global struct {
		VCenterIP           string `gcfg:"ip"`
		ClusterID           string `gcfg:"cluster-id"`
		ClusterDistribution string `gcfg:"cluster-distribution"`
		CAFile              string `gcfg:"ca-file"`
	}
	// Virtual Center configurations
	VirtualCenter map[string]*VirtualCenterConfig
}

type VirtualCenterConfig struct {
	User         string `gcfg:"user"`
	Password     string `gcfg:"password"`
	VCenterPort  string `gcfg:"port"`
	InsecureFlag bool   `gcfg:"insecure-flag"`
	Datacenters  string `gcfg:"datacenters"`
}
