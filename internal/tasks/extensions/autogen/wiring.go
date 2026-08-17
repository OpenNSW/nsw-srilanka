package autogen

import (
	"errors"
	"fmt"

	"github.com/OpenNSW/core/taskflow/extensions"
	"github.com/OpenNSW/nsw-srilanka/internal/autogen"
)

// Register installs the agency reference autogenerator extension into the task extensions registry.
func Register(reg *extensions.Registry, seqService autogen.SequenceService) error {
	if reg == nil {
		return errors.New("autogen extension: registry is nil")
	}
	if seqService == nil {
		return errors.New("autogen extension: sequence service is nil")
	}

	if err := reg.Register(ExtAgencyRefGen, NewExtension(seqService)); err != nil {
		return fmt.Errorf("autogen extension: register %s: %w", ExtAgencyRefGen, err)
	}
	return nil
}
