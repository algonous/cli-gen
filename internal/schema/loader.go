package schema

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

func LoadCLIFile(path string) (*CliFile, error) {
	var out CliFile
	if err := decodeYAML(path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func LoadActionFile(path string) (*ActionFile, error) {
	var out ActionFile
	if err := decodeYAML(path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func decodeYAML(path string, out any) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func LoadSchemaDir(dir string) (*SchemaSet, error) {
	cliPath := filepath.Join(dir, "cli.yaml")
	if _, err := os.Stat(cliPath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("cli.yaml not found")
		}
		return nil, fmt.Errorf("stat cli.yaml: %w", err)
	}

	cliFile, err := LoadCLIFile(cliPath)
	if err != nil {
		return nil, err
	}

	actionsDir := filepath.Join(dir, "actions")
	actionEntries, err := os.ReadDir(actionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return &SchemaSet{CLI: cliFile, Actions: []*ActionFile{}}, nil
		}
		return nil, fmt.Errorf("read actions dir: %w", err)
	}

	var yamlNames []string
	for _, entry := range actionEntries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) == ".yaml" {
			yamlNames = append(yamlNames, entry.Name())
		}
	}
	sort.Strings(yamlNames)

	actions := make([]*ActionFile, 0, len(yamlNames))
	for _, name := range yamlNames {
		actionPath := filepath.Join(actionsDir, name)
		action, err := LoadActionFile(actionPath)
		if err != nil {
			return nil, err
		}
		actions = append(actions, action)
	}

	return &SchemaSet{CLI: cliFile, Actions: actions}, nil
}

func CheckDuplicateActions(set *SchemaSet) error {
	seen := map[string]int{}
	for i, action := range set.Actions {
		if prev, ok := seen[action.Name]; ok {
			return fmt.Errorf("duplicate action name %q at indexes %d and %d", action.Name, prev, i)
		}
		seen[action.Name] = i
	}
	return nil
}
