package models

import "time"

type Book struct {
	ID          int       `json:"id"`
	Title       string    `json:"title"`
	AuthorName  string    `json:"author_name"`
	IsAvailable bool      `json:"is_available"`
	RentPrice   float64   `json:"rent_price"` // Price per day or flat rate
	CreatedAt   time.Time `json:"created_at"`
}
