package oblodai

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// names.lock is the generator's: it adds the names a new contract brings and refuses to drop one.
// names.2.0.txt is the list of 2.0.0, written once and never regenerated: MIGRATION-2.0.md is a
// document of that release, so it is checked against the frozen list, not the growing lock.

func nameList(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(raw))
}

func TestNamesLockIsTheClientSurface(t *testing.T) {
	lock := nameList(t, "names.lock")
	if len(lock) != len(Routes) {
		t.Fatalf("names.lock has %d names, the client %d operations: regenerate (make sdk in the backend)", len(lock), len(Routes))
	}
	for _, name := range nameList(t, "names.2.0.txt") {
		if !slices.Contains(lock, name) {
			t.Errorf("names.lock dropped %s of 2.0: a breaking change", name)
		}
	}
}

func TestMigrationMapsEveryNameOf20(t *testing.T) {
	raw, err := os.ReadFile("MIGRATION-2.0.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	frozen := nameList(t, "names.2.0.txt")
	for _, name := range frozen {
		if !strings.Contains(text, "| `"+name+"` |") {
			t.Errorf("MIGRATION-2.0.md does not map %s", name)
		}
	}
	row := regexp.MustCompile("(?m)^\\| [^|]+ \\| `[A-Za-z.]+` \\| `([a-z_]+\\.[a-z_0-9]+)` \\|$")
	for _, m := range row.FindAllStringSubmatch(text, -1) {
		if !slices.Contains(frozen, m[1]) {
			t.Errorf("MIGRATION-2.0.md maps %s, which is not a name of 2.0", m[1])
		}
	}
}
