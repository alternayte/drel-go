package users

import (
	"fmt"

	"github.com/alternayte/drel"
)

type BalanceTransferred struct {
	FromID int
	ToID   int
	Amount int
}

type User struct {
	drel.Model[int]
	name    string `db:"name"`
	balance int    `db:"balance"`
}

func NewUser(name string, balance int) *User {
	return &User{name: name, balance: balance}
}

func (u *User) Name() string { return u.name }
func (u *User) Balance() int { return u.balance }

func (u *User) Transfer(amount int, to *User) error {
	if u.balance < amount {
		return fmt.Errorf("insufficient balance: have %d, need %d", u.balance, amount)
	}
	u.balance -= amount
	to.balance += amount
	u.RecordEvent(BalanceTransferred{FromID: u.ID(), ToID: to.ID(), Amount: amount})
	return nil
}
