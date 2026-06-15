package models

import (
	"database/sql/driver"
	"fmt"
	"strings"

	"github.com/alternayte/drel"
)

// Email is a string-backed single-column value object (sql.Scanner + driver.Valuer).
type Email struct{ address string }

func NewEmail(s string) (Email, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if !strings.Contains(s, "@") {
		return Email{}, fmt.Errorf("invalid email: %q", s)
	}
	return Email{address: s}, nil
}

func (e Email) String() string { return e.address }
func (e Email) IsZero() bool   { return e.address == "" }

func (e Email) Value() (driver.Value, error) {
	if e.address == "" {
		return nil, nil
	}
	return e.address, nil
}

func (e *Email) Scan(src any) error {
	if src == nil {
		e.address = ""
		return nil
	}
	s, ok := src.(string)
	if !ok {
		return fmt.Errorf("Email.Scan: expected string, got %T", src)
	}
	e.address = s
	return nil
}

// Cents is an int64-backed single-column value object. It would silently get a
// TEXT column before the W2-G2 VOBaseType fix.
type Cents struct{ n int64 }

func NewCents(n int64) Cents { return Cents{n: n} }
func (c Cents) Int64() int64 { return c.n }

func (c Cents) Value() (driver.Value, error) { return c.n, nil }

func (c *Cents) Scan(src any) error {
	switch v := src.(type) {
	case int64:
		c.n = v
	case nil:
		c.n = 0
	default:
		return fmt.Errorf("Cents.Scan: expected int64, got %T", src)
	}
	return nil
}

// UserAccount demonstrates single-column value objects (Email, Cents).
// It was renamed from Account to avoid a duplicate DB field name with
// accounts.Account (which demonstrates multi-column VOs).
type UserAccount struct {
	drel.Model[int]
	email   Email `db:"email,unique"`
	balance Cents `db:"balance"`
}

func NewUserAccount(email Email, balance Cents) *UserAccount {
	return &UserAccount{email: email, balance: balance}
}

func (a *UserAccount) Email() Email   { return a.email }
func (a *UserAccount) Balance() Cents { return a.balance }

func (a *UserAccount) SetBalance(c Cents) { a.balance = c }
