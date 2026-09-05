package domain

import (
	"errors"
	"fmt"
	"strings"
)

// ComponentKind identifies categories of runtime infrastructure components.
type ComponentKind string

const (
	ComponentKindDatabase ComponentKind = "database"
	ComponentKindRiver    ComponentKind = "river"
	ComponentKindNATS     ComponentKind = "nats"
	ComponentKindAdapter  ComponentKind = "adapter"
	ComponentKindHTTP     ComponentKind = "http"
)

// ComponentStatus represents the health/readiness fact of a runtime component.
type ComponentStatus struct {
	Name     string
	Kind     ComponentKind
	Ready    bool
	Required bool
	Detail   string
}

// EvaluateReadiness verifies that all required components are healthy.
func EvaluateReadiness(components []ComponentStatus) error {
	var unready []string
	for _, comp := range components {
		if comp.Required && !comp.Ready {
			detail := comp.Detail
			if detail == "" {
				detail = "not ready"
			}
			unready = append(unready, fmt.Sprintf("%s (%s: %s)", comp.Name, comp.Kind, detail))
		}
	}
	if len(unready) > 0 {
		return fmt.Errorf("%w: %s", ErrComponentsUnready, strings.Join(unready, ", "))
	}
	return nil
}

// ErrComponentsUnready is returned when one or more required runtime components fail readiness evaluation.
var ErrComponentsUnready = errors.New("required runtime components are not ready")
