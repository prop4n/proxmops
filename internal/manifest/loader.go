package manifest

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

// yamlExts are the file extensions treated as manifests.
var yamlExts = map[string]bool{".yaml": true, ".yml": true}

// Load walks root within fsys and decodes every YAML manifest it finds into a
// concrete Resource. A file may hold multiple documents separated by "---".
// Each resource is validated; the first failure aborts the load.
func Load(fsys fs.FS, root string) ([]Resource, error) {
	var resources []Resource
	err := fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !yamlExts[strings.ToLower(path.Ext(p))] {
			return nil
		}
		found, err := decodeFile(fsys, p)
		if err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		resources = append(resources, found...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return resources, nil
}

// decodeFile decodes every YAML document in a single file.
func decodeFile(fsys fs.FS, p string) ([]Resource, error) {
	f, err := fsys.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var resources []Resource
	dec := yaml.NewDecoder(f)
	for {
		var node yaml.Node
		if err := dec.Decode(&node); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		res, err := decodeNode(&node)
		if err != nil {
			return nil, err
		}
		if err := res.Validate(); err != nil {
			return nil, fmt.Errorf("invalid %s %q: %w", res.GetTypeMeta().Kind, res.GetObjectMeta().Name, err)
		}
		resources = append(resources, res)
	}
	return resources, nil
}

// decodeNode reads the kind from a decoded YAML node and decodes it into the
// matching concrete resource type.
func decodeNode(node *yaml.Node) (Resource, error) {
	var probe struct {
		TypeMeta `yaml:",inline"`
	}
	if err := node.Decode(&probe); err != nil {
		return nil, err
	}
	switch probe.Kind {
	case KindVirtualMachine:
		return decodeInto[VirtualMachine](node)
	case KindContainer:
		return decodeInto[Container](node)
	case KindIso:
		return decodeInto[Iso](node)
	case KindNetwork:
		return decodeInto[Network](node)
	case KindTemplate:
		return decodeInto[Template](node)
	case "":
		return nil, fmt.Errorf("missing kind")
	default:
		return nil, fmt.Errorf("unknown kind %q", probe.Kind)
	}
}

// decodeInto decodes a node into T and returns it as a Resource.
func decodeInto[T Resource](node *yaml.Node) (Resource, error) {
	var v T
	if err := node.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}
