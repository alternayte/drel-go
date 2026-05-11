package models

import "github.com/alternayte/drel"

type Task struct {
	drel.Model[int]
	Title    string `db:"title"`
	Done     bool   `db:"done"`
	Priority int    `db:"priority"`
}

func NewTask(title string, priority int) *Task {
	return &Task{Title: title, Priority: priority}
}

func (t *Task) MarkDone() {
	t.Done = true
}

func (t *Task) SetPriority(p int) {
	t.Priority = p
}
