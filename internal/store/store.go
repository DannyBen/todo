package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/dannyben/todo/internal/task"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
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
		`CREATE TABLE IF NOT EXISTS task_references (
			task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
			reference_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
			PRIMARY KEY (task_id, reference_id),
			CHECK (task_id <> reference_id)
		)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			return nil, fmt.Errorf("initialize todo database: %w", err)
		}
	}
	return &Store{db: db}, nil
}

func (store *Store) Close() error {
	return store.db.Close()
}

func (store *Store) Add(description string, tags []string, references []int64) (task.Task, error) {
	tx, err := store.db.BeginTx(context.Background(), nil)
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
	tx, err := store.db.BeginTx(context.Background(), nil)
	if err != nil {
		return task.Task{}, fmt.Errorf("begin edit: %w", err)
	}
	defer tx.Rollback()

	if err := requireTask(tx, id); err != nil {
		return task.Task{}, err
	}
	if change.Description != nil {
		if *change.Description == "" {
			return task.Task{}, fmt.Errorf("task description cannot be empty")
		}
		if _, err := tx.Exec("UPDATE tasks SET description = ? WHERE id = ?", *change.Description, id); err != nil {
			return task.Task{}, fmt.Errorf("update task %d: %w", id, err)
		}
	}
	if err := removeTags(tx, id, change.RemoveTags); err != nil {
		return task.Task{}, err
	}
	if err := addTags(tx, id, change.AddTags); err != nil {
		return task.Task{}, err
	}
	if err := removeReferences(tx, id, change.RemoveReferences); err != nil {
		return task.Task{}, err
	}
	if err := addReferences(tx, id, change.AddReferences); err != nil {
		return task.Task{}, err
	}
	if err := tx.Commit(); err != nil {
		return task.Task{}, fmt.Errorf("commit edit: %w", err)
	}
	return store.Get(id)
}

func (store *Store) Delete(id int64) error {
	result, err := store.db.Exec("DELETE FROM tasks WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete task %d: %w", id, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("confirm deletion of task %d: %w", id, err)
	}
	if changed == 0 {
		return fmt.Errorf("task %d not found", id)
	}
	return nil
}

func (store *Store) Get(id int64) (task.Task, error) {
	var item task.Task
	err := store.db.QueryRow("SELECT id, description FROM tasks WHERE id = ?", id).Scan(&item.ID, &item.Description)
	if errors.Is(err, sql.ErrNoRows) {
		return task.Task{}, fmt.Errorf("task %d not found", id)
	}
	if err != nil {
		return task.Task{}, fmt.Errorf("read task %d: %w", id, err)
	}
	if err := store.loadDetails(&item); err != nil {
		return task.Task{}, err
	}
	return item, nil
}

func (store *Store) List() ([]task.Task, error) {
	rows, err := store.db.Query("SELECT id, description FROM tasks ORDER BY id")
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
		if err := store.loadDetails(&tasks[index]); err != nil {
			return nil, err
		}
	}
	return tasks, nil
}

func (store *Store) loadDetails(item *task.Task) error {
	tagRows, err := store.db.Query("SELECT tag FROM tags WHERE task_id = ? ORDER BY tag", item.ID)
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

	referenceRows, err := store.db.Query("SELECT reference_id FROM task_references WHERE task_id = ? ORDER BY reference_id", item.ID)
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

func requireTask(tx *sql.Tx, id int64) error {
	var exists int
	if err := tx.QueryRow("SELECT 1 FROM tasks WHERE id = ?", id).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("task %d not found", id)
	} else if err != nil {
		return fmt.Errorf("read task %d: %w", id, err)
	}
	return nil
}

func addTags(tx *sql.Tx, id int64, tags []string) error {
	for _, tag := range tags {
		if _, err := tx.Exec("INSERT OR IGNORE INTO tags (task_id, tag) VALUES (?, ?)", id, tag); err != nil {
			return fmt.Errorf("add tag +%s to task %d: %w", tag, id, err)
		}
	}
	return nil
}

func removeTags(tx *sql.Tx, id int64, tags []string) error {
	for _, tag := range tags {
		if _, err := tx.Exec("DELETE FROM tags WHERE task_id = ? AND tag = ?", id, tag); err != nil {
			return fmt.Errorf("remove tag +%s from task %d: %w", tag, id, err)
		}
	}
	return nil
}

func addReferences(tx *sql.Tx, id int64, references []int64) error {
	for _, reference := range references {
		if reference == id {
			return fmt.Errorf("task %d cannot reference itself", id)
		}
		if err := requireTask(tx, reference); err != nil {
			return fmt.Errorf("add reference @%d to task %d: %w", reference, id, err)
		}
		if _, err := tx.Exec("INSERT OR IGNORE INTO task_references (task_id, reference_id) VALUES (?, ?)", id, reference); err != nil {
			return fmt.Errorf("add reference @%d to task %d: %w", reference, id, err)
		}
	}
	return nil
}

func removeReferences(tx *sql.Tx, id int64, references []int64) error {
	for _, reference := range references {
		if _, err := tx.Exec("DELETE FROM task_references WHERE task_id = ? AND reference_id = ?", id, reference); err != nil {
			return fmt.Errorf("remove reference @%d from task %d: %w", reference, id, err)
		}
	}
	return nil
}
