// Example: relationships
//
// Demonstrates eager loading with Include using split queries:
// has_many (Author -> Books), has_one (Author -> Profile),
// and belongs_to (Book -> Author).
//
// Models use `rel` struct tags, and `drel generate` emits RelationInfo
// and IncludeSpec variables in the db/ package for cross-package support.
//
// Usage:
//
//	cd examples/relationships
//	go run ../../cmd/drel generate
//	export DATABASE_URL="postgres://localhost:5432/drelexample?sslmode=disable"
//	go run .
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/alternayte/drel"
	"github.com/alternayte/drel/examples/relationships/db"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://localhost:5432/drelexample?sslmode=disable"
	}

	ctx := context.Background()
	database, err := db.Open(dsn, drel.WithContext(ctx))
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer database.Close()

	setup(ctx, database)
	defer teardown(ctx, database)

	// === has_many: Author -> Books ===
	fmt.Println("=== Author with Books (has_many) ===")
	alice, err := database.Authors.Include(db.AuthorIncludeBooks).Find(ctx, 1)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  %s has %d books:\n", alice.Name, len(alice.Books))
	for _, b := range alice.Books {
		fmt.Printf("    - %s\n", b.Title)
	}

	// === has_one: Author -> Profile ===
	fmt.Println("\n=== Author with Profile (has_one) ===")
	bob, err := database.Authors.Include(db.AuthorIncludeProfile).Find(ctx, 2)
	if err != nil {
		log.Fatal(err)
	}
	if bob.Profile != nil {
		fmt.Printf("  %s's bio: %s\n", bob.Name, bob.Profile.Bio)
	}

	// === belongs_to: Book -> Author ===
	fmt.Println("\n=== Book with Author (belongs_to) ===")
	book, err := database.Books.Include(db.BookIncludeAuthor).Find(ctx, 1)
	if err != nil {
		log.Fatal(err)
	}
	if book.Author != nil {
		fmt.Printf("  \"%s\" by %s\n", book.Title, book.Author.Name)
	}

	// === All authors with books ===
	fmt.Println("\n=== All Authors with Books ===")
	allAuthors, err := database.Authors.Include(db.AuthorIncludeBooks).All(ctx)
	if err != nil {
		log.Fatal(err)
	}
	for _, a := range allAuthors {
		fmt.Printf("  %s (%d books)\n", a.Name, len(a.Books))
	}
}

func setup(ctx context.Context, database *db.DB) {
	database.Exec(ctx, `DROP TABLE IF EXISTS books`)
	database.Exec(ctx, `DROP TABLE IF EXISTS author_profiles`)
	database.Exec(ctx, `DROP TABLE IF EXISTS authors`)
	database.Exec(ctx, `
		CREATE TABLE authors (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	database.Exec(ctx, `
		CREATE TABLE books (
			id SERIAL PRIMARY KEY,
			title TEXT NOT NULL,
			author_id INTEGER NOT NULL REFERENCES authors(id),
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	database.Exec(ctx, `
		CREATE TABLE author_profiles (
			id SERIAL PRIMARY KEY,
			bio TEXT NOT NULL,
			author_id INTEGER NOT NULL UNIQUE REFERENCES authors(id),
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)

	database.Exec(ctx, `INSERT INTO authors (name) VALUES ('Alice'), ('Bob'), ('Carol')`)
	database.Exec(ctx, `INSERT INTO books (title, author_id) VALUES ('Go in Practice', 1), ('Advanced Go', 1), ('Learning Go', 1), ('Python Basics', 2)`)
	database.Exec(ctx, `INSERT INTO author_profiles (bio, author_id) VALUES ('Go expert and author', 1), ('Polyglot programmer', 2)`)
}

func teardown(ctx context.Context, database *db.DB) {
	database.Exec(ctx, `DROP TABLE IF EXISTS books`)
	database.Exec(ctx, `DROP TABLE IF EXISTS author_profiles`)
	database.Exec(ctx, `DROP TABLE IF EXISTS authors`)
}
