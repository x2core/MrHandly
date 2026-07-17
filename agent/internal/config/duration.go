package config

import "time"

// Duration is a time.Duration that decodes from a TOML string like "1s" or
// "500ms". TOML has no native duration type, so the config uses a string and
// this wrapper parses it via encoding.TextUnmarshaler.
type Duration time.Duration

// UnmarshalText implements encoding.TextUnmarshaler for the TOML decoder.
func (d *Duration) UnmarshalText(text []byte) error {
	v, err := time.ParseDuration(string(text))
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

// Std returns the value as a standard time.Duration.
func (d Duration) Std() time.Duration { return time.Duration(d) }

func (d Duration) String() string { return time.Duration(d).String() }
