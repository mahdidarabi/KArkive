package pipeline

import (
	"strings"
	"testing"
)

func TestCommonScriptEmbedded(t *testing.T) {
	s := CommonScript()
	if s == "" {
		t.Fatal("common.sh is empty")
	}
	for _, fn := range []string{"log()", "wait_for()", "hold_until_job_done()", "mark_failed()", "pipeline_init()"} {
		if !strings.Contains(s, fn) {
			t.Errorf("common.sh missing %s", fn)
		}
	}
}

func TestBackupScriptsEmbedded(t *testing.T) {
	for _, name := range []string{"cleanup.sh", "pgdump.sh", "mysqldump.sh", "redisdump.sh", "compress.sh", "encrypt.sh", "s3-sync.sh"} {
		s, err := BackupScript(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if s == "" {
			t.Fatalf("%s is empty", name)
		}
		assertComposedOnce(t, name, s)
	}
}

func TestRestoreScriptsEmbedded(t *testing.T) {
	for _, name := range []string{"cleanup.sh", "fetch.sh", "decrypt.sh", "extract.sh", "pgrestore.sh", "mysqlrestore.sh", "redisrestore.sh"} {
		s, err := RestoreScript(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if s == "" {
			t.Fatalf("%s is empty", name)
		}
		assertComposedOnce(t, name, s)
	}
}

func assertComposedOnce(t *testing.T, name, s string) {
	t.Helper()
	if !strings.HasPrefix(s, CommonScript()) {
		t.Errorf("%s: expected common.sh to be prepended", name)
	}
	for _, fn := range []string{"log() {", "wait_for() {", "hold_until_job_done() {", "mark_failed() {"} {
		if n := strings.Count(s, fn); n != 1 {
			t.Errorf("%s: want one %q, got %d", name, fn, n)
		}
	}
}
