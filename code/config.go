package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type config struct {
	SourceAccountConnectString string `yaml:"sourceAccountConnectString,omitempty"`
	SourceContainerName        string `yaml:"sourceContainerName,omitempty"`
	DestAccountConnectString   string `yaml:"destAccountConnectString,omitempty"`
	DestinationContainerName   string `yaml:"destinationContainerName,omitempty"`
}

func NewConfigFromConfigFile(file string) (*config, error) {
	conf := new(config)
	f, err := os.ReadFile(file)
	if err != nil {
		return &config{}, fmt.Errorf("unable to open configuration file : %v", err)
	}
	err = yaml.Unmarshal(f, &conf)
	if err != nil {
		return &config{}, fmt.Errorf("unable to read configuration from configuration file : %v", err)
	}
	return conf, nil
}

func (c *config) GetSourceAccountConnectString() string {
	return c.SourceAccountConnectString
}

func (c *config) GetSourceContainerName() string {
	return c.SourceContainerName
}

func (c *config) GetDestAccountConnectString() string {
	return c.DestAccountConnectString
}

func (c *config) GetDestinationContainerName() string {
	return c.DestinationContainerName
}
