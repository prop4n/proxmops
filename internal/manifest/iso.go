package manifest

import "fmt"

// Iso is the desired state of an ISO image or template synced onto a storage.
type Iso struct {
	TypeMeta `yaml:",inline"`
	Metadata ObjectMeta `yaml:"metadata"`
	Spec     IsoSpec    `yaml:"spec"`
}

// IsoSpec describes where an image comes from and where it should live.
type IsoSpec struct {
	Source   string   `yaml:"source"`
	Node     string   `yaml:"node"`
	Storage  string   `yaml:"storage"`
	Checksum Checksum `yaml:"checksum,omitempty"`
}

// Checksum verifies the integrity of a downloaded image.
type Checksum struct {
	Algo  string `yaml:"algo"`
	Value string `yaml:"value"`
}

// GetTypeMeta implements Resource.
func (i Iso) GetTypeMeta() TypeMeta { return i.TypeMeta }

// GetObjectMeta implements Resource.
func (i Iso) GetObjectMeta() ObjectMeta { return i.Metadata }

// Validate reports whether the manifest is well-formed.
func (i Iso) Validate() error {
	if err := validateMeta(i.TypeMeta, i.Metadata); err != nil {
		return err
	}
	if i.Spec.Source == "" {
		return fmt.Errorf("spec.source is required")
	}
	if i.Spec.Storage == "" {
		return fmt.Errorf("spec.storage is required")
	}
	return nil
}
