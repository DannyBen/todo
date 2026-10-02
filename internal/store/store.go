package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/dannyben/todo/internal/task"
	_ "modernc.org/sqlite"
)

type Store struct {
	db   *sql.DB
	path string
}

type executor interface {
	queryer
	Exec(string, ...any) (sql.Result, error)
}

type queryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

func Open(path string) (*Store, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}
	db, err := sql.Open("sqlite", databaseURI(path, "_txlock", "immediate"))
	if err != nil {
		return nil, fmt.Errorf("open todo database: %w", err)
	}
	db.SetMaxOpenConns(1)

	for _, statement := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		`CREATE TABLE IF NOT EXISTS tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			description TEXT NOT NULL CHECK (description <> '')
		)`,
		`CREATE TABLE IF NOT EXISTS tags (
			task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
			tag TEXT NOT NULL,
			PRIMARY KEY (task_id, tag)
		)`,
		`CREATE TABLE IF NOT EXISTS task_connections (
			task_id_a INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
			task_id_b INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
			PRIMARY KEY (task_id_a, task_id_b),
			CHECK (task_id_a < task_id_b)
		)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			return nil, fmt.Errorf("initialize todo database: %w", err)
		}
	}
	if err := migrateReferences(db); err != nil {
		db.Close()
		return nil, err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("resolve database path: %w", err)
	}
	return &Store{db: db, path: path}, nil
}

func migrateReferences(db *sql.DB) error {
	var exists int
	err := db.QueryRow(`SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'task_references'`).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect legacy references: %w", err)
	}
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("begin reference migration: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`
		INSERT OR IGNORE INTO task_connections (task_id_a, task_id_b)
		SELECT
			CASE WHEN task_id < reference_id THEN task_id ELSE reference_id END,
			CASE WHEN task_id < reference_id THEN reference_id ELSE task_id END
		FROM task_references
		WHERE task_id <> reference_id
	`); err != nil {
		return fmt.Errorf("migrate references: %w", err)
	}
	if _, err := tx.Exec("DROP TABLE task_references"); err != nil {
		return fmt.Errorf("remove legacy references: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reference migration: %w", err)
	}
	return nil
}

func (store *Store) Close() error {
	return store.db.Close()
}

func (store *Store) Add(description string, tags []string, references []int64) (task.Task, error) {
	tx, err := store.beginMutation()
	if err != nil {
		return task.Task{}, fmt.Errorf("begin add: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.Exec("INSERT INTO tasks (description) VALUES (?)", description)
	if err != nil {
		return task.Task{}, fmt.Errorf("add task: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return task.Task{}, fmt.Errorf("read task ID: %w", err)
	}
	if err := addTags(tx, id, tags); err != nil {
		return task.Task{}, err
	}
	if err := addReferences(tx, id, references); err != nil {
		return task.Task{}, err
	}
	if err := tx.Commit(); err != nil {
		return task.Task{}, fmt.Errorf("commit add: %w", err)
	}
	return store.Get(id)
}

func (store *Store) Edit(id int64, change task.Change) (task.Task, error) {
	tx, err := store.beginMutation()
	if err != nil {
		return task.Task{}, fmt.Errorf("begin edit: %w", err)
	}
	defer tx.Rollback()

	if err := requireTask(tx, id); err != nil {
		return task.Task{}, err
	}
	if err := applyChange(tx, id, change); err != nil {
		return task.Task{}, err
	}
	if err := tx.Commit(); err != nil {
		return task.Task{}, fmt.Errorf("commit edit: %w", err)
	}
	return store.Get(id)
}

func (store *Store) EditMany(ids []int64, change task.Change) ([]task.Task, error) {
	if len(ids) > 1 && change.Description != nil {
		return nil, fmt.Errorf("bulk edits cannot replace task descriptions; use only tag and connection operations")
	}
	tx, err := store.beginMutation()
	if err != nil {
		return nil, fmt.Errorf("begin multi-task edit: %w", err)
	}
	defer tx.Rollback()

	for _, id := range ids {
		if err := requireTask(tx, id); err != nil {
			return nil, err
		}
	}
	for _, id := range ids {
		if err := applyChange(tx, id, change); err != nil {
			return nil, err
		}
	}

	updated := make([]task.Task, 0, len(ids))
	for _, id := range ids {
		item, err := get(tx, id)
		if err != nil {
			return nil, err
		}
		updated = append(updated, item)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit multi-task edit: %w", err)
	}
	return updated, nil
}

func applyChange(tx executor, id int64, change task.Change) error {
	if change.Description != nil {
		if *change.Description == "" {
			return fmt.Errorf("task description cannot be empty")
		}
		if _, err := tx.Exec("UPDATE tasks SET description = ? WHERE id = ?", *change.Description, id); err != nil {
			return fmt.Errorf("update task %d: %w", id, err)
		}
	}
	if err := removeTags(tx, id, change.RemoveTags); err != nil {
		return err
	}
	if err := addTags(tx, id, change.AddTags); err != nil {
		return err
	}
	if err := removeReferences(tx, id, change.RemoveReferences); err != nil {
		return err
	}
	if err := addReferences(tx, id, change.AddReferences); err != nil {
		return err
	}
	return nil
}

func (store *Store) Delete(id int64) (task.Task, error) {
	tx, err := store.beginMutation()
	if err != nil {
		return task.Task{}, fmt.Errorf("begin delete: %w", err)
	}
	defer tx.Rollback()

	item, err := get(tx, id)
	if err != nil {
		return task.Task{}, err
	}
	result, err := tx.Exec("DELETE FROM tasks WHERE id = ?", id)
	if err != nil {
		return task.Task{}, fmt.Errorf("delete task %d: %w", id, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return task.Task{}, fmt.Errorf("confirm deletion of task %d: %w", id, err)
	}
	if changed == 0 {
		return task.Task{}, fmt.Errorf("task %d not found", id)
	}
	if err := tx.Commit(); err != nil {
		return task.Task{}, fmt.Errorf("commit delete: %w", err)
	}
	return item, nil
}

func (store *Store) DeleteMany(ids []int64) ([]task.Task, error) {
	tx, err := store.beginMutation()
	if err != nil {
		return nil, fmt.Errorf("begin multi-task deletion: %w", err)
	}
	defer tx.Rollback()

	deleted := make([]task.Task, 0, len(ids))
	for _, id := range ids {
		item, err := get(tx, id)
		if err != nil {
			return nil, err
		}
		deleted = append(deleted, item)
	}
	for _, id := range ids {
		result, err := tx.Exec("DELETE FROM tasks WHERE id = ?", id)
		if err != nil {
			return nil, fmt.Errorf("delete task %d: %w", id, err)
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("confirm deletion of task %d: %w", id, err)
		}
		if changed != 1 {
			return nil, fmt.Errorf("task %d disappeared during multi-task deletion", id)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit multi-task deletion: %w", err)
	}
	return deleted, nil
}

func (store *Store) Get(id int64) (task.Task, error) {
	return get(store.db, id)
}

func get(source queryer, id int64) (task.Task, error) {
	var item task.Task
	err := source.QueryRow("SELECT id, description FROM tasks WHERE id = ?", id).Scan(&item.ID, &item.Description)
	if errors.Is(err, sql.ErrNoRows) {
		return task.Task{}, fmt.Errorf("task %d not found", id)
	}
	if err != nil {
		return task.Task{}, fmt.Errorf("read task %d: %w", id, err)
	}
	if err := loadDetails(source, &item); err != nil {
		return task.Task{}, err
	}
	return item, nil
}

func (store *Store) List() ([]task.Task, error) {
	return list(store.db)
}

func list(source queryer) ([]task.Task, error) {
	rows, err := source.Query("SELECT id, description FROM tasks ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	var tasks []task.Task
	for rows.Next() {
		var item task.Task
		if err := rows.Scan(&item.ID, &item.Description); err != nil {
			rows.Close()
			return nil, fmt.Errorf("read task: %w", err)
		}
		tasks = append(tasks, item)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close task list: %w", err)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read task list: %w", err)
	}
	for index := range tasks {
		if err := loadDetails(source, &tasks[index]); err != nil {
			return nil, err
		}
	}
	return tasks, nil
}

func loadDetails(source queryer, item *task.Task) error {
	tagRows, err := source.Query("SELECT tag FROM tags WHERE task_id = ? ORDER BY tag", item.ID)
	if err != nil {
		return fmt.Errorf("read tags for task %d: %w", item.ID, err)
	}
	for tagRows.Next() {
		var tag string
		if err := tagRows.Scan(&tag); err != nil {
			tagRows.Close()
			return fmt.Errorf("read tag for task %d: %w", item.ID, err)
		}
		item.Tags = append(item.Tags, tag)
	}
	if err := tagRows.Close(); err != nil {
		return fmt.Errorf("close tags for task %d: %w", item.ID, err)
	}
	if err := tagRows.Err(); err != nil {
		return fmt.Errorf("read tags for task %d: %w", item.ID, err)
	}

	referenceRows, err := source.Query(`
		SELECT CASE WHEN task_id_a = ? THEN task_id_b ELSE task_id_a END
		FROM task_connections
		WHERE task_id_a = ? OR task_id_b = ?
		ORDER BY 1
	`, item.ID, item.ID, item.ID)
	if err != nil {
		return fmt.Errorf("read references for task %d: %w", item.ID, err)
	}
	defer referenceRows.Close()
	for referenceRows.Next() {
		var id int64
		if err := referenceRows.Scan(&id); err != nil {
			return fmt.Errorf("read reference for task %d: %w", item.ID, err)
		}
		item.References = append(item.References, id)
	}
	if err := referenceRows.Err(); err != nil {
		return fmt.Errorf("read references for task %d: %w", item.ID, err)
	}
	return nil
}

func requireTask(tx queryer, id int64) error {
	var exists int
	if err := tx.QueryRow("SELECT 1 FROM tasks WHERE id = ?", id).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("task %d not found", id)
	} else if err != nil {
		return fmt.Errorf("read task %d: %w", id, err)
	}
	return nil
}

func addTags(tx executor, id int64, tags []string) error {
	for _, tag := range tags {
		if _, err := tx.Exec("INSERT OR IGNORE INTO tags (task_id, tag) VALUES (?, ?)", id, tag); err != nil {
			return fmt.Errorf("add tag +%s to task %d: %w", tag, id, err)
		}
	}
	return nil
}

func removeTags(tx executor, id int64, tags []string) error {
	for _, tag := range tags {
		if _, err := tx.Exec("DELETE FROM tags WHERE task_id = ? AND tag = ?", id, tag); err != nil {
			return fmt.Errorf("remove tag +%s from task %d: %w", tag, id, err)
		}
	}
	return nil
}

func addReferences(tx executor, id int64, references []int64) error {
	for _, reference := range references {
		if reference == id {
			return fmt.Errorf("task %d cannot reference itself", id)
		}
		if err := requireTask(tx, reference); err != nil {
			return fmt.Errorf("add connection +%d to task %d: %w", reference, id, err)
		}
		first, second := connectionIDs(id, reference)
		if _, err := tx.Exec("INSERT OR IGNORE INTO task_connections (task_id_a, task_id_b) VALUES (?, ?)", first, second); err != nil {
			return fmt.Errorf("add connection +%d to task %d: %w", reference, id, err)
		}
	}
	return nil
}

func removeReferences(tx executor, id int64, references []int64) error {
	for _, reference := range references {
		first, second := connectionIDs(id, reference)
		if _, err := tx.Exec("DELETE FROM task_connections WHERE task_id_a = ? AND task_id_b = ?", first, second); err != nil {
			return fmt.Errorf("remove connection -%d from task %d: %w", reference, id, err)
		}
	}
	return nil
}

func connectionIDs(first, second int64) (int64, int64) {
	if first < second {
		return first, second
	}
	return second, first
}
