package models

import "github.com/alternayte/drel"

type Author struct {
	drel.Model[int]
	Name    string `db:"name"`
	Books   []*Book
	Profile *AuthorProfile
}

type Book struct {
	drel.Model[int]
	Title    string `db:"title"`
	AuthorID int    `db:"author_id"`
	Author   *Author
}

type AuthorProfile struct {
	drel.Model[int]
	Bio      string `db:"bio"`
	AuthorID int    `db:"author_id"`
}
