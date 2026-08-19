package pipeline

import "testing"

func TestBackupScriptsEmbedded(t *testing.T) {
	for _, name := range []string{"cleanup.sh", "pgdump.sh", "compress.sh", "encrypt.sh", "s3-sync.sh"} {
		s, err := BackupScript(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if s == "" {
			t.Fatalf("%s is empty", name)
		}
	}
}
