package models

import "time"

type User struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Password  string    `json:"-"`       // Hidden from JSON responses for security
	Balance   float64   `json:"balance"` // Wallet balance to pay rent
	CreatedAt time.Time `json:"created_at"`
}

type BorrowRecord struct {
	ID         int        `json:"id"`
	UserID     int        `json:"user_id"`
	BookID     int        `json:"book_id"`
	BorrowedAt time.Time  `json:"borrowed_at"`
	DueDate    time.Time  `json:"due_date"`
	ReturnedAt *time.Time `json:"returned_at,omitempty"` // Pointer handles nullable database fields
	RentPaid   float64    `json:"rent_paid"`
}
