package cmds

import "github.com/netswitcher/netswitcher/internal/paths"

// defaultConfigPath / defaultStatePath delegate to internal/paths so commands
// don't repeat the %ProgramData% logic.
func defaultConfigPath() (string, error) { return paths.ConfigPath() }
func defaultStatePath() (string, error)  { return paths.StatePath() }
