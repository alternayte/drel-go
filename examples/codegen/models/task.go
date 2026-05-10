package models

import "github.com/alternayte/drel"

type Task struct {
	drel.Model[int]
	title    string `db:"title"`
	done     bool   `db:"done"`
	priority int    `db:"priority"`
}

func NewTask(title string, priority int) *Task {
	return &Task{title: title, priority: priority}
}

func (t *Task) Title() string  { return t.title }
func (t *Task) Done() bool     { return t.done }
func (t *Task) Priority() int  { return t.priority }

func (t *Task) MarkDone() {
	t.done = true
}

func (t *Task) SetPriority(p int) {
	t.priority = p
}
