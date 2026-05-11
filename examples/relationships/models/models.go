package models

import "github.com/alternayte/drel"

type Author struct {
	drel.Model[int]
	Name    string         `db:"name"`
	Books   []*Book        `rel:"has_many,fk=author_id"`
	Profile *AuthorProfile `rel:"has_one,fk=author_id"`
}

type Book struct {
	drel.Model[int]
	Title    string  `db:"title"`
	AuthorID int     `db:"author_id"`
	Author   *Author `rel:"belongs_to,fk=author_id"`
}

type AuthorProfile struct {
	drel.Model[int]
	Bio      string `db:"bio"`
	AuthorID int    `db:"author_id"`
}
