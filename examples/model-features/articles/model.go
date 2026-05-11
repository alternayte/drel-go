package articles

import "github.com/alternayte/drel"

type Article struct {
	drel.Model[int]
	drel.SoftDelete
	drel.Versioned
	drel.Audit
	Title string `db:"title"`
	Body  string `db:"body"`
}
