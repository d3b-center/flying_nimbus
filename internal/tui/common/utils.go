package common

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

const DefaultLabelWidth = 18

var (
	DefaultLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("245"))

	DefaultValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255"))
)

type KVOption func(*kvConfig)

type kvConfig struct {
	labelStyle lipgloss.Style
	valueStyle lipgloss.Style
	labelWidth int
}

func KV(label, value string, opts ...KVOption) string {
	cfg := kvConfig{
		labelStyle: DefaultLabelStyle,
		valueStyle: DefaultValueStyle,
		labelWidth: DefaultLabelWidth,
	}

	for _, opt := range opts {
		opt(&cfg)
	}

	return fmt.Sprintf(
		"%s %s",
		cfg.labelStyle.Render(fmt.Sprintf("%-*s", cfg.labelWidth, label+":")),
		cfg.valueStyle.Render(value),
	)
}

func WithLabelStyle(s lipgloss.Style) KVOption {
	return func(c *kvConfig) {
		c.labelStyle = s
	}
}

func WithValueStyle(s lipgloss.Style) KVOption {
	return func(c *kvConfig) {
		c.valueStyle = s
	}
}

func WithLabelWidth(w int) KVOption {
	return func(c *kvConfig) {
		c.labelWidth = w
	}
}
