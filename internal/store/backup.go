package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dannyben/todo/internal/task"
	"modernc.org/sqlite"
)

const backupTimeFormat = "20060102-150405.000000000"

type backup struct {
	dir     string
	prefix  string
	count   int
	pending string
	before  []task.Task
}

type mutation struct {
	*sql.Tx
	backup *backup
}

func databaseURI(path, key, value string) string {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return (&url.URL{Scheme: "file", Path: path, RawQuery: url.Values{key: {value}}.Encode()}).String()
}

func (store *Store) beginMutation() (*mutation, error) {
	tx, err := store.db.BeginTx(context.Background(), nil)
	if err != nil {
		return nil, err
	}
	write := &mutation{Tx: tx}
	write.backup, err = store.prepareBackup(tx)
	if err != nil {
		write.Rollback()
		return nil, fmt.Errorf("backup: %w", err)
	}
	return write, nil
}

func (store *Store) prepareBackup(tx *sql.Tx) (_ *backup, resultErr error) {
	dir, count, err := parseBackupSetting(os.Getenv("TODO_BACKUP"))
	if err != nil {
		return nil, err
	}
	if dir == "" {
		return nil, nil
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(filepath.Dir(store.path), dir)
	}
	if err := ownBackupDirectory(dir, store.path); err != nil {
		return nil, err
	}
	before, err := list(tx)
	if err != nil {
		return nil, err
	}
	file, err := os.CreateTemp(dir, ".pending-*")
	if err != nil {
		return nil, fmt.Errorf("create snapshot: %w", err)
	}
	pending := file.Name()
	defer func() {
		if resultErr != nil {
			os.Remove(pending)
		}
	}()
	if err := file.Close(); err != nil {
		return nil, err
	}
	if err := copyDatabase(store.path, pending); err != nil {
		return nil, fmt.Errorf("copy database: %w", err)
	}
	file, err = os.OpenFile(pending, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	err = file.Sync()
	closeErr := file.Close()
	if err := errors.Join(err, closeErr); err != nil {
		return nil, fmt.Errorf("save snapshot: %w", err)
	}
	return &backup{dir: dir, prefix: filepath.Base(filepath.Dir(store.path)) + "-", count: count, pending: pending, before: before}, nil
}

func parseBackupSetting(value string) (string, int, error) {
	if value == "on" {
		return ".todo", 10, nil
	}
	prefix, path, found := strings.Cut(value, "@")
	number := prefix
	if strings.HasPrefix(number, "+") || strings.HasPrefix(number, "-") {
		number = number[1:]
	}
	if !found || number == "" {
		return value, 10, nil
	}
	for _, character := range number {
		if character < '0' || character > '9' {
			return value, 10, nil
		}
	}
	count, err := strconv.Atoi(prefix)
	if err != nil || count < 1 {
		return "", 0, fmt.Errorf("TODO_BACKUP count must be a positive integer")
	}
	if path == "" {
		return "", 0, fmt.Errorf("TODO_BACKUP path must not be empty")
	}
	return path, count, nil
}

func ownBackupDirectory(dir, database string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	owner := filepath.Join(dir, ".backupid")
	info, err := os.Lstat(owner)
	if err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s must be a regular file", owner)
		}
		contents, err := os.ReadFile(owner)
		if err != nil {
			return fmt.Errorf("read ownership: %w", err)
		}
		if string(contents) != database+"\n" {
			return fmt.Errorf("directory %s belongs to a different database", dir)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read ownership: %w", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read directory: %w", err)
	}
	if len(entries) != 0 {
		return fmt.Errorf("directory %s is not empty and has no .backupid", dir)
	}
	file, err := os.OpenFile(owner, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("claim directory: %w", err)
	}
	_, writeErr := file.WriteString(database + "\n")
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		os.Remove(owner)
		return fmt.Errorf("save ownership: %w", err)
	}
	return nil
}

// The mutation's IMMEDIATE transaction excludes other writers. A separate
// reader copies the committed state before the mutation starts changing it.
func copyDatabase(source, destination string) error {
	db, err := sql.Open("sqlite", databaseURI(source, "mode", "ro"))
	if err != nil {
		return err
	}
	defer db.Close()
	conn, err := db.Conn(context.Background())
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Raw(func(driverConn any) error {
		copier := driverConn.(interface {
			NewBackup(string) (*sqlite.Backup, error)
		})
		snapshot, err := copier.NewBackup(databaseURI(destination, "mode", "rw"))
		if err != nil {
			return err
		}
		_, stepErr := snapshot.Step(-1)
		return errors.Join(stepErr, snapshot.Finish())
	})
}

func (write *mutation) Rollback() error {
	if write.backup != nil && write.backup.pending != "" {
		os.Remove(write.backup.pending)
	}
	return write.Tx.Rollback()
}

func (write *mutation) Commit() error {
	if write.backup == nil {
		return write.Tx.Commit()
	}
	backup := write.backup
	after, err := list(write.Tx)
	if err != nil {
		return err
	}
	if reflect.DeepEqual(backup.before, after) {
		if err := os.Remove(backup.pending); err != nil {
			return fmt.Errorf("remove unused snapshot: %w", err)
		}
		backup.pending = ""
		return write.Tx.Commit()
	}
	name := backup.prefix + time.Now().UTC().Format(backupTimeFormat) + ".sqlite"
	destination := filepath.Join(backup.dir, name)
	// Reserve the final name so a timestamp collision cannot overwrite a backup.
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("reserve backup name: %w", err)
	}
	if err := file.Close(); err != nil {
		os.Remove(destination)
		return err
	}
	if err := os.Rename(backup.pending, destination); err != nil {
		os.Remove(destination)
		return fmt.Errorf("publish backup: %w", err)
	}
	backup.pending = destination
	if err := write.Tx.Commit(); err != nil {
		return err
	}
	backup.pending = ""
	if err := backup.prune(); err != nil {
		return fmt.Errorf("change saved and backup created, but retention failed: %w", err)
	}
	return nil
}

func (backup *backup) prune() error {
	entries, err := os.ReadDir(backup.dir)
	if err != nil {
		return err
	}
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if !entry.Type().IsRegular() || !strings.HasPrefix(name, backup.prefix) || !strings.HasSuffix(name, ".sqlite") {
			continue
		}
		stamp := strings.TrimSuffix(strings.TrimPrefix(name, backup.prefix), ".sqlite")
		if _, err := time.Parse(backupTimeFormat, stamp); err == nil {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for index := 0; index < len(names)-backup.count; index++ {
		if err := os.Remove(filepath.Join(backup.dir, names[index])); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
