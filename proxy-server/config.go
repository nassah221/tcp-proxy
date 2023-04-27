package main

import (
	"encoding/json"
	"io"
	"os"
)

type Config struct {
	Apps []struct {
		Name    string   `json:"Name"`
		Ports   []int    `json:"Ports"`
		Targets []string `json:"Targets"`
	}
}

func LoadConfig(path string) (*Config, error) {
	var config Config

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	b, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(b, &config); err != nil {
		return nil, err
	}

	return &config, nil
}
