package models

import "github.com/alternayte/drel"

type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
	RoleGuest Role = "guest"
)

type Priority int

const (
	PriorityLow    Priority = 0
	PriorityMedium Priority = 1
	PriorityHigh   Priority = 2
)

type Account struct {
	drel.Model[int]
	Name     string   `db:"name"`
	Role     Role     `db:"role,default=user"`
	Priority Priority `db:"priority"`
}
