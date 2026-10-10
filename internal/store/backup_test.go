package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dannyben/todo/internal/task"
)

func backupFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".sqlite") {
			files = append(files, filepath.Join(dir, entry.Name()))
		} else if strings.HasPrefix(entry.Name(), ".pending-") {
			t.Fatalf("temporary snapshot leaked: %s", entry.Name())
		}
	}
	return files
}

func openBackupStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("TODO_BACKUP", ".todo")
	database, err := Open(filepath.Join(dir, ".todo.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return database, filepath.Join(dir, ".todo")
}

func TestBackupRestoresCompleteDatabase(t *testing.T) {
	database, dir := openBackupStore(t)
	first, err := database.Add("First", []string{"ready"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := database.Add("Second", []string{"done"}, []int64{first.ID})
	if err != nil {
		t.Fatal(err)
	}
	before, err := database.List()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.DeleteMany([]int64{first.ID, second.ID}); err != nil {
		t.Fatal(err)
	}
	files := backupFiles(t, dir)
	if len(files) != 3 {
		t.Fatalf("backups = %v", files)
	}
	owner, err := os.ReadFile(filepath.Join(dir, ".backupid"))
	if err != nil || string(owner) != database.path+"\n" {
		t.Fatalf("owner = %q, err=%v", owner, err)
	}
	if !strings.HasPrefix(filepath.Base(files[2]), filepath.Base(filepath.Dir(database.path))+"-") {
		t.Fatalf("unrecognizable backup name: %s", files[2])
	}
	readOnly, err := sql.Open("sqlite", databaseURI(files[2], "mode", "ro"))
	if err != nil {
		t.Fatal(err)
	}
	var integrity string
	err = readOnly.QueryRow("PRAGMA integrity_check").Scan(&integrity)
	readOnly.Close()
	if err != nil || integrity != "ok" {
		t.Fatalf("snapshot integrity=%q, err=%v", integrity, err)
	}
	contents, err := os.ReadFile(files[2])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Add("Later", nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(database.path, contents, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TODO_BACKUP", "")
	restored, err := Open(database.path)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	after, err := restored.List()
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("restored tasks=%#v, want=%#v, err=%v", after, before, err)
	}
	third, err := restored.Add("Third", nil, nil)
	if err != nil || third.ID != 3 {
		t.Fatalf("restored ID sequence: task=%#v err=%v", third, err)
	}
}

func TestBackupRetention(t *testing.T) {
	for _, test := range []struct {
		count string
		want  int
	}{
		{"", 10}, {"2", 2}, {"30", 12},
	} {
		t.Run("count="+test.count, func(t *testing.T) {
			database, dir := openBackupStore(t)
			if test.count != "" {
				t.Setenv("TODO_BACKUP", test.count+"@.todo")
			}
			for index := 0; index < 12; index++ {
				if _, err := database.Add(fmt.Sprintf("Task %d", index), nil, nil); err != nil {
					t.Fatal(err)
				}
			}
			files := backupFiles(t, dir)
			if len(files) != test.want {
				t.Fatalf("got %d backups, want %d", len(files), test.want)
			}
			oldest, err := sql.Open("sqlite", databaseURI(files[0], "mode", "ro"))
			if err != nil {
				t.Fatal(err)
			}
			var tasks int
			err = oldest.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&tasks)
			oldest.Close()
			if err != nil || tasks != 12-test.want {
				t.Fatalf("oldest snapshot contains %d tasks, err=%v", tasks, err)
			}
		})
	}
}

func TestParseBackupSetting(t *testing.T) {
	for _, test := range []struct {
		value string
		path  string
		count int
	}{
		{"", "", 10},
		{"on", ".todo", 10},
		{".todo", ".todo", 10},
		{"./on", "./on", 10},
		{"10@.todo", ".todo", 10},
		{"30@/tmp/todo backups", "/tmp/todo backups", 30},
		{"backups@home", "backups@home", 10},
		{"./10@backups", "./10@backups", 10},
		{"30@backups@home", "backups@home", 30},
	} {
		path, count, err := parseBackupSetting(test.value)
		if err != nil || path != test.path || count != test.count {
			t.Errorf("%q: path=%q count=%d err=%v", test.value, path, count, err)
		}
	}
	for _, value := range []string{"0@.todo", "-1@.todo", "10@", "999999999999999999999999999@.todo"} {
		if _, _, err := parseBackupSetting(value); err == nil {
			t.Errorf("invalid setting %q was accepted", value)
		}
	}
}

func TestBackupFailuresAndNoChangesPreserveHistory(t *testing.T) {
	database, dir := openBackupStore(t)
	item, err := database.Add("First", []string{"ready"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	before := backupFiles(t, dir)
	description := item.Description
	for _, change := range []task.Change{
		{}, {Description: &description}, {AddTags: []string{"ready"}},
		{RemoveTags: []string{"missing"}}, {AddTags: []string{"ready"}, RemoveTags: []string{"ready"}},
	} {
		if _, err := database.Edit(item.ID, change); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.EditMany([]int64{item.ID}, task.Change{AddTags: []string{"ready"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := database.DeleteMany(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Add("Failed", nil, []int64{99}); err == nil {
		t.Fatal("invalid add succeeded")
	}
	if _, err := database.Edit(item.ID, task.Change{AddTags: []string{"changed"}, AddReferences: []int64{99}}); err == nil {
		t.Fatal("invalid edit succeeded")
	}
	if _, err := database.DeleteMany([]int64{item.ID, 99}); err == nil {
		t.Fatal("invalid delete succeeded")
	}
	if _, err := database.Delete(99); err == nil {
		t.Fatal("invalid single delete succeeded")
	}
	if _, err := database.List(); err != nil {
		t.Fatal(err)
	}
	if after := backupFiles(t, dir); !reflect.DeepEqual(before, after) {
		t.Fatalf("history changed: before=%v after=%v", before, after)
	}
	current, err := database.Get(item.ID)
	if err != nil || !reflect.DeepEqual(current, item) {
		t.Fatalf("failed edit changed task: %#v err=%v", current, err)
	}
}

func TestBackupDirectoryOwnership(t *testing.T) {
	for _, setup := range []string{"absent", "empty", "unowned", "matching", "mismatch", "marker directory"} {
		t.Run(setup, func(t *testing.T) {
			database, dir := openBackupStore(t)
			if setup != "absent" {
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			switch setup {
			case "unowned":
				if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			case "matching", "mismatch":
				owner := database.path
				if setup == "mismatch" {
					owner += ".other"
				}
				if err := os.WriteFile(filepath.Join(dir, ".backupid"), []byte(owner+"\n"), 0600); err != nil {
					t.Fatal(err)
				}
			case "marker directory":
				if err := os.Mkdir(filepath.Join(dir, ".backupid"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			_, err := database.Add("First", nil, nil)
			allowed := setup == "absent" || setup == "empty" || setup == "matching"
			if (err == nil) != allowed {
				t.Fatalf("allowed=%v err=%v", allowed, err)
			}
			items, err := database.List()
			if err != nil || (len(items) == 1) != allowed {
				t.Fatalf("tasks=%#v err=%v", items, err)
			}
		})
	}
}

func TestBackupsCheckOwnershipEveryTime(t *testing.T) {
	database, dir := openBackupStore(t)
	if _, err := database.Add("First", nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, ".backupid")); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Add("Second", nil, nil); err == nil {
		t.Fatal("unowned nonempty directory was accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, ".backupid")); !os.IsNotExist(err) {
		t.Fatalf("missing owner was recreated: %v", err)
	}
	if len(backupFiles(t, dir)) != 1 {
		t.Fatal("failed mutation changed backups")
	}
}

func TestBackupDisabledAndInvalidCounts(t *testing.T) {
	database, dir := openBackupStore(t)
	t.Setenv("TODO_BACKUP", "")
	if _, err := database.Add("First", nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("disabled backup created directory: %v", err)
	}
	t.Setenv("TODO_BACKUP", ".todo")
	for _, count := range []string{"0", "-1", "999999999999999999999999999999"} {
		t.Setenv("TODO_BACKUP", count+"@.todo")
		if _, err := database.Add("Failed", nil, nil); err == nil {
			t.Errorf("invalid count %q was accepted", count)
		}
	}
	items, err := database.List()
	if err != nil || len(items) != 1 {
		t.Fatalf("invalid configuration changed tasks: %#v err=%v", items, err)
	}
}

func TestBackupFromWALDatabase(t *testing.T) {
	database, dir := openBackupStore(t)
	if _, err := database.db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Add("First", nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Delete(1); err != nil {
		t.Fatal(err)
	}
	files := backupFiles(t, dir)
	snapshot, err := sql.Open("sqlite", databaseURI(files[len(files)-1], "mode", "ro"))
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	var description string
	if err := snapshot.QueryRow("SELECT description FROM tasks WHERE id = 1").Scan(&description); err != nil || description != "First" {
		t.Fatalf("WAL backup description=%q err=%v", description, err)
	}
}

func TestBackupDirectoryCannotBeShared(t *testing.T) {
	database, dir := openBackupStore(t)
	t.Setenv("TODO_BACKUP", dir)
	if _, err := database.Add("First", nil, nil); err != nil {
		t.Fatal(err)
	}
	other, err := Open(filepath.Join(t.TempDir(), ".todo.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, err := other.Add("Other", nil, nil); err == nil || !strings.Contains(err.Error(), "different database") {
		t.Fatalf("shared backup directory error=%v", err)
	}
	items, err := other.List()
	if err != nil || len(items) != 0 || len(backupFiles(t, dir)) != 1 {
		t.Fatalf("rejected ownership changed tasks or backups: tasks=%v err=%v", items, err)
	}
}

func TestBackupRetentionPreservesOtherFiles(t *testing.T) {
	database, dir := openBackupStore(t)
	t.Setenv("TODO_BACKUP", "1@.todo")
	if _, err := database.Add("First", nil, nil); err != nil {
		t.Fatal(err)
	}
	name := filepath.Base(filepath.Dir(database.path)) + "-manual.sqlite"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Add("Second", nil, nil); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil || string(contents) != "keep" {
		t.Fatalf("retention changed unrelated file: contents=%q err=%v", contents, err)
	}
}

func TestBackupAndMutationExcludeOtherWriters(t *testing.T) {
	database, dir := openBackupStore(t)
	if _, err := database.Add("First", nil, nil); err != nil {
		t.Fatal(err)
	}
	other, err := Open(database.path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, err := other.db.Exec("PRAGMA busy_timeout=0"); err != nil {
		t.Fatal(err)
	}
	write, err := database.beginMutation()
	if err != nil {
		t.Fatal(err)
	}
	defer write.Rollback()
	if competing, err := other.db.BeginTx(context.Background(), nil); err == nil {
		competing.Rollback()
		t.Fatal("another writer entered between backup and mutation")
	}
	if _, err := write.Exec("INSERT INTO tasks (description) VALUES ('Second')"); err != nil {
		t.Fatal(err)
	}
	if err := write.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := other.Add("Third", nil, nil); err != nil {
		t.Fatal(err)
	}
	files := backupFiles(t, dir)
	snapshot, err := sql.Open("sqlite", databaseURI(files[len(files)-1], "mode", "ro"))
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	var count int
	if err := snapshot.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&count); err != nil || count != 2 {
		t.Fatalf("next writer's backup missed committed changes: count=%d err=%v", count, err)
	}
}
