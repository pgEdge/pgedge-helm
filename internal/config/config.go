package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// NodeBootstrap describes how a node should be bootstrapped.
type NodeBootstrap struct {
	Mode       string `yaml:"mode"`
	SourceNode string `yaml:"sourceNode"`
}

// Node represents a pgEdge Spock node from the Helm config.
type Node struct {
	Name             string        `yaml:"name"`
	Hostname         string        `yaml:"hostname"`
	InternalHostname string        `yaml:"internalHostname"`
	Bootstrap        NodeBootstrap `yaml:"bootstrap"`
}

// Config holds all configuration for the init-spock job.
type Config struct {
	AppName    string
	DBName     string
	Namespace  string
	AdminUser  string
	PgEdgeUser string
	ResetSpock bool
	Nodes      []Node
}

// LoadNodes reads node definitions from a YAML file.
func LoadNodes(path string) ([]Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read nodes config: %w", err)
	}
	var nodes []Node
	if err := yaml.Unmarshal(data, &nodes); err != nil {
		return nil, fmt.Errorf("parse nodes config: %w", err)
	}
	return nodes, nil
}

// Load reads the full job configuration from env vars and the node YAML file.
func Load(nodesPath string) (*Config, error) {
	appName := os.Getenv("APP_NAME")
	if appName == "" {
		return nil, fmt.Errorf("APP_NAME environment variable is required")
	}
	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		return nil, fmt.Errorf("DB_NAME environment variable is required")
	}
	namespace := os.Getenv("NAMESPACE")
	if namespace == "" {
		namespace = "default"
	}
	resetSpock, _ := strconv.ParseBool(os.Getenv("RESET_SPOCK"))
	nodes, err := LoadNodes(nodesPath)
	if err != nil {
		return nil, err
	}
	return &Config{
		AppName:    appName,
		DBName:     dbName,
		Namespace:  namespace,
		AdminUser:  "admin",
		PgEdgeUser: "pgedge",
		ResetSpock: resetSpock,
		Nodes:      nodes,
	}, nil
}
