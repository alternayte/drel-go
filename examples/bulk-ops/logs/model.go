package logs

import "github.com/alternayte/drel"

type LogEntry struct {
	drel.Model[int]
	Level   string `db:"level"`
	Message string `db:"message"`
}
