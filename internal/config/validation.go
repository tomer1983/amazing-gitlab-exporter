package config

import (
	"fmt"

	"github.com/go-playground/validator/v10"
)

// Validate validates the configuration using struct tags registered with
// the go-playground/validator library.
func Validate(cfg *Config) error {
	v := validator.New()
	if err := v.Struct(cfg); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}
	return nil
}
