package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// ValidateYAML rejects configuration fields that cannot decode exactly into Config.
func ValidateYAML(data []byte) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return errors.New("decode YAML configuration: invalid YAML")
	}

	var extra yaml.Node
	if err := decoder.Decode(&extra); err == nil {
		return errors.New("configuration must contain exactly one YAML document")
	} else if !errors.Is(err, io.EOF) {
		return errors.New("decode YAML configuration: invalid YAML")
	}

	if len(document.Content) == 0 {
		return nil
	}
	return validateYAMLNode(document.Content[0], reflect.TypeFor[Config](), "")
}

// validateYAMLNode checks one YAML node against its exact configuration type.
func validateYAMLNode(node *yaml.Node, target reflect.Type, path string) error {
	if node.Kind == yaml.AliasNode {
		return yamlFieldError(path, "must not use YAML aliases")
	}
	if target == reflect.TypeFor[time.Duration]() {
		if err := requireYAMLScalar(node, path, "!!str", "a duration string"); err != nil {
			return err
		}
		if _, err := time.ParseDuration(node.Value); err != nil {
			return yamlFieldError(path, "must be a valid duration string")
		}
		return nil
	}

	switch target.Kind() { //nolint:exhaustive // Config intentionally supports only the field kinds below.
	case reflect.Struct:
		return validateYAMLMapping(node, target, path)
	case reflect.Slice:
		return validateYAMLSequence(node, target, path)
	case reflect.String:
		return requireYAMLScalar(node, path, "!!str", "a string")
	case reflect.Int, reflect.Int64:
		if err := requireYAMLScalar(node, path, "!!int", "an integer"); err != nil {
			return err
		}
		value := reflect.New(target)
		if err := node.Decode(value.Interface()); err != nil {
			return yamlFieldError(path, "must be a valid integer")
		}
		return nil
	default:
		return fmt.Errorf("validate configuration field %q: unsupported type %s", path, target)
	}
}

// validateYAMLMapping checks a configuration struct represented as a YAML mapping.
func validateYAMLMapping(node *yaml.Node, target reflect.Type, path string) error {
	const mappingPairSize = 2

	if node.Kind != yaml.MappingNode {
		return yamlFieldError(path, "must be a mapping")
	}
	fields := mapstructureFields(target)
	seen := make(map[string]struct{}, len(node.Content)/mappingPairSize)
	for pair := range len(node.Content) / mappingPairSize {
		index := pair * mappingPairSize
		key := node.Content[index]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
			return yamlFieldError(path, "must use string field names")
		}
		fieldType, ok := fields[key.Value]
		fieldPath := joinYAMLPath(path, key.Value)
		if !ok {
			return fmt.Errorf("unknown configuration field %q", fieldPath)
		}
		if _, ok := seen[key.Value]; ok {
			return fmt.Errorf("configuration field %q appears more than once", fieldPath)
		}
		seen[key.Value] = struct{}{}
		if err := validateYAMLNode(node.Content[index+1], fieldType, fieldPath); err != nil {
			return err
		}
	}
	return nil
}

// validateYAMLSequence checks one list-valued configuration field.
func validateYAMLSequence(node *yaml.Node, target reflect.Type, path string) error {
	if node.Kind != yaml.SequenceNode {
		return yamlFieldError(path, "must be a sequence")
	}
	for index, item := range node.Content {
		itemPath := fmt.Sprintf("%s[%d]", path, index)
		if err := validateYAMLNode(item, target.Elem(), itemPath); err != nil {
			return err
		}
	}
	return nil
}

// mapstructureFields returns the exact configured names and types for a struct.
func mapstructureFields(target reflect.Type) map[string]reflect.Type {
	fields := make(map[string]reflect.Type, target.NumField())
	for field := range target.Fields() {
		name, _, _ := strings.Cut(field.Tag.Get("mapstructure"), ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = field.Name
		}
		fields[name] = field.Type
	}
	return fields
}

// requireYAMLScalar checks a scalar node's resolved YAML type.
func requireYAMLScalar(node *yaml.Node, path string, tag string, expected string) error {
	if node.Kind != yaml.ScalarNode || node.Tag != tag {
		return yamlFieldError(path, "must be "+expected)
	}
	return nil
}

// yamlFieldError formats a field-safe decoding error without including its value.
func yamlFieldError(path string, message string) error {
	if path == "" {
		return fmt.Errorf("configuration document %s", message)
	}
	return fmt.Errorf("configuration field %q %s", path, message)
}

// joinYAMLPath joins one nested configuration field name.
func joinYAMLPath(parent string, child string) string {
	if parent == "" {
		return child
	}
	return parent + "." + child
}
