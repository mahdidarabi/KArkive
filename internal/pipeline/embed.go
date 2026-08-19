package pipeline

import (
	"embed"
	"fmt"
)

//go:embed scripts/backup/*.sh
var backupScripts embed.FS

// BackupScript returns the named file from scripts/backup/.
func BackupScript(name string) (string, error) {
	b, err := backupScripts.ReadFile("scripts/backup/" + name)
	if err != nil {
		return "", fmt.Errorf("embedded backup script %s: %w", name, err)
	}
	return string(b), nil
}

// MustBackupScript is BackupScript and panics on a missing file (compile-time embed).
func MustBackupScript(name string) string {
	s, err := BackupScript(name)
	if err != nil {
		panic(err)
	}
	return s
}
