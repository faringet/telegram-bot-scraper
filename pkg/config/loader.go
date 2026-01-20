package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Validatable interface{ Validate() error }

type Options struct {
	Paths         []string
	Names         []string
	Type          string
	EnvPrefix     string
	OptionalFiles bool
}

func Load[T any](opts Options) (T, error) {
	var zero T
	var cfg T

	v := viper.New()
	typ := opts.Type
	if typ == "" {
		typ = "yaml"
	}
	v.SetConfigType(typ)

	foundAny := false
	fv := viper.New()
	fv.SetConfigType(typ)

	for _, p := range opts.Paths {
		if p != "" {
			fv.AddConfigPath(p)
		}
	}

	for _, name := range opts.Names {
		if name == "" {
			continue
		}
		fv.SetConfigName(name)
		if err := fv.ReadInConfig(); err == nil {
			if err := v.MergeConfigMap(fv.AllSettings()); err != nil {
				return zero, fmt.Errorf("merge %s: %w", name, err)
			}
			foundAny = true
		}
	}

	if !foundAny && !opts.OptionalFiles && len(opts.Names) > 0 {
		return zero, fmt.Errorf("config files not found in %v for names %v", opts.Paths, opts.Names)
	}

	if opts.EnvPrefix != "" {
		v.SetEnvPrefix(opts.EnvPrefix)
	}
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.Unmarshal(&cfg); err != nil {
		return zero, fmt.Errorf("unmarshal config: %w", err)
	}

	if vv, ok := any(&cfg).(Validatable); ok {
		if err := vv.Validate(); err != nil {
			return zero, fmt.Errorf("invalid config: %w", err)
		}
	}

	return cfg, nil
}

func MustLoad[T any](opts Options) *T {
	cfg, err := Load[T](opts)
	if err != nil {
		panic(err)
	}
	return &cfg
}
