// Example: relationships
//
// Demonstrates eager loading with Include using split queries:
// has_many (Author → Books), has_one (Author → Profile),
// and belongs_to (Book → Author).
//
// Usage:
//
//	export DATABASE_URL="postgres://localhost:5432/drelexample?sslmode=disable"
//	go run ./examples/relationships/
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/alternayte/drel"
)

// --- Models ---

type Author struct {
	ID        int
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
	Books     []*Book
	Profile   *AuthorProfile
}

type Book struct {
	ID        int
	Title     string
	AuthorID  int
	CreatedAt time.Time
	UpdatedAt time.Time
	Author    *Author
}

type AuthorProfile struct {
	ID        int
	Bio       string
	AuthorID  int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// --- Metadata ---

var AuthorMeta = drel.ModelMeta[Author]{
	Table:    "authors",
	Columns:  []string{"id", "name", "created_at", "updated_at"},
	PKColumn: "id",
	Scan: func(row drel.Row) (*Author, error) {
		a := &Author{}
		return a, row.Scan(&a.ID, &a.Name, &a.CreatedAt, &a.UpdatedAt)
	},
	PKValue: func(a *Author) any { return a.ID },
	ColumnValue: func(a *Author, idx int) any {
		switch idx {
		case 0:
			return a.ID
		case 1:
			return a.Name
		}
		return nil
	},
}

var BookMeta = drel.ModelMeta[Book]{
	Table:    "books",
	Columns:  []string{"id", "title", "author_id", "created_at", "updated_at"},
	PKColumn: "id",
	Scan: func(row drel.Row) (*Book, error) {
		b := &Book{}
		return b, row.Scan(&b.ID, &b.Title, &b.AuthorID, &b.CreatedAt, &b.UpdatedAt)
	},
	PKValue: func(b *Book) any { return b.ID },
	ColumnValue: func(b *Book, idx int) any {
		switch idx {
		case 0:
			return b.ID
		case 1:
			return b.Title
		case 2:
			return b.AuthorID
		}
		return nil
	},
}

var ProfileMeta = drel.ModelMeta[AuthorProfile]{
	Table:    "author_profiles",
	Columns:  []string{"id", "bio", "author_id", "created_at", "updated_at"},
	PKColumn: "id",
	Scan: func(row drel.Row) (*AuthorProfile, error) {
		p := &AuthorProfile{}
		return p, row.Scan(&p.ID, &p.Bio, &p.AuthorID, &p.CreatedAt, &p.UpdatedAt)
	},
	PKValue: func(p *AuthorProfile) any { return p.ID },
	ColumnValue: func(p *AuthorProfile, idx int) any {
		switch idx {
		case 0:
			return p.ID
		case 2:
			return p.AuthorID
		}
		return nil
	},
}

// --- Relationship definitions ---

var booksRelation = drel.RelationInfo{
	Name:     "books",
	Type:     drel.HasMany,
	FKColumn: "author_id",
	FieldSetter: func(parent any, related any) {
		a := parent.(*Author)
		if items, ok := related.([]any); ok {
			a.Books = make([]*Book, len(items))
			for i, item := range items {
				a.Books[i] = item.(*Book)
			}
		}
	},
}

var profileRelation = drel.RelationInfo{
	Name:     "profile",
	Type:     drel.HasOne,
	FKColumn: "author_id",
	FieldSetter: func(parent any, related any) {
		a := parent.(*Author)
		if p, ok := related.(*AuthorProfile); ok {
			a.Profile = p
		}
	},
}

var authorRelation = drel.RelationInfo{
	Name:     "author",
	Type:     drel.BelongsTo,
	FKColumn: "author_id",
	FieldSetter: func(parent any, related any) {
		b := parent.(*Book)
		if a, ok := related.(*Author); ok {
			b.Author = a
		}
	},
}

func initRelations() {
	booksRelation.RelatedMeta = drel.ToMetaBase(&BookMeta)
	profileRelation.RelatedMeta = drel.ToMetaBase(&ProfileMeta)
	authorRelation.RelatedMeta = drel.ToMetaBase(&AuthorMeta)
}

func main() {
	initRelations()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://localhost:5432/drelexample?sslmode=disable"
	}

	ctx := context.Background()
	engine, err := drel.NewEngine(dsn, drel.WithContext(ctx))
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer engine.Close()

	setup(ctx, engine)
	defer teardown(ctx, engine)

	authors := drel.NewRepository(engine, AuthorMeta)
	books := drel.NewRepository(engine, BookMeta)

	// === has_many: Author → Books ===
	fmt.Println("=== Author with Books (has_many) ===")
	alice, err := authors.Include(drel.NewIncludeSpec(&booksRelation)).Find(ctx, 1)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  %s has %d books:\n", alice.Name, len(alice.Books))
	for _, b := range alice.Books {
		fmt.Printf("    - %s\n", b.Title)
	}

	// === has_one: Author → Profile ===
	fmt.Println("\n=== Author with Profile (has_one) ===")
	bob, err := authors.Include(drel.NewIncludeSpec(&profileRelation)).Find(ctx, 2)
	if err != nil {
		log.Fatal(err)
	}
	if bob.Profile != nil {
		fmt.Printf("  %s's bio: %s\n", bob.Name, bob.Profile.Bio)
	}

	// === belongs_to: Book → Author ===
	fmt.Println("\n=== Book with Author (belongs_to) ===")
	book, err := books.Include(drel.NewIncludeSpec(&authorRelation)).Find(ctx, 1)
	if err != nil {
		log.Fatal(err)
	}
	if book.Author != nil {
		fmt.Printf("  \"%s\" by %s\n", book.Title, book.Author.Name)
	}

	// === All authors with books ===
	fmt.Println("\n=== All Authors with Books ===")
	allAuthors, err := authors.Include(drel.NewIncludeSpec(&booksRelation)).All(ctx)
	if err != nil {
		log.Fatal(err)
	}
	for _, a := range allAuthors {
		fmt.Printf("  %s (%d books)\n", a.Name, len(a.Books))
	}
}

func setup(ctx context.Context, engine *drel.Engine) {
	drv := engine.Driver()
	drv.Exec(ctx, `DROP TABLE IF EXISTS books`)
	drv.Exec(ctx, `DROP TABLE IF EXISTS author_profiles`)
	drv.Exec(ctx, `DROP TABLE IF EXISTS authors`)
	drv.Exec(ctx, `
		CREATE TABLE authors (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	drv.Exec(ctx, `
		CREATE TABLE books (
			id SERIAL PRIMARY KEY,
			title TEXT NOT NULL,
			author_id INTEGER NOT NULL REFERENCES authors(id),
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	drv.Exec(ctx, `
		CREATE TABLE author_profiles (
			id SERIAL PRIMARY KEY,
			bio TEXT NOT NULL,
			author_id INTEGER NOT NULL UNIQUE REFERENCES authors(id),
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)

	drv.Exec(ctx, `INSERT INTO authors (name) VALUES ('Alice'), ('Bob'), ('Carol')`)
	drv.Exec(ctx, `INSERT INTO books (title, author_id) VALUES ('Go in Practice', 1), ('Advanced Go', 1), ('Learning Go', 1), ('Python Basics', 2)`)
	drv.Exec(ctx, `INSERT INTO author_profiles (bio, author_id) VALUES ('Go expert and author', 1), ('Polyglot programmer', 2)`)
}

func teardown(ctx context.Context, engine *drel.Engine) {
	drv := engine.Driver()
	drv.Exec(ctx, `DROP TABLE IF EXISTS books`)
	drv.Exec(ctx, `DROP TABLE IF EXISTS author_profiles`)
	drv.Exec(ctx, `DROP TABLE IF EXISTS authors`)
}
