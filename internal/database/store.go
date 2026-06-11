package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/ashok-shasmal/library-portal/internal/models"
)

type Store struct {
	DB      *sql.DB
	Timeout time.Duration
}

func NewStore(db *sql.DB) *Store {
	return &Store{DB: db, Timeout: 5 * time.Second}
}

// --- Users ---
func (s *Store) CreateUser(ctx context.Context, u *models.User) error {
	log.Printf("Creating user: %s (%s)", u.Name, u.Email) // Debug log
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now()
	}

	query := `INSERT INTO users (name, email, password, balance, created_at) VALUES ($1,$2,$3,$4,$5) RETURNING id`
	if err := s.DB.QueryRowContext(ctx, query, u.Name, u.Email, u.Password, u.Balance, u.CreatedAt).Scan(&u.ID); err != nil {
		log.Printf("Error creating user: %v", err.Error()) // Debug log
		return err
	}
	log.Printf("User created with ID: %d", u.ID) // Debug log
	return nil
}

func (s *Store) GetUserByID(ctx context.Context, id int) (*models.User, error) {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	u := &models.User{}
	query := `SELECT id, name, email, password, balance, created_at FROM users WHERE id = $1`
	row := s.DB.QueryRowContext(ctx, query, id)
	if err := row.Scan(&u.ID, &u.Name, &u.Email, &u.Password, &u.Balance, &u.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return u, nil
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	u := &models.User{}
	query := `SELECT id, name, email, password, balance, created_at FROM users WHERE email = $1`
	row := s.DB.QueryRowContext(ctx, query, email)
	if err := row.Scan(&u.ID, &u.Name, &u.Email, &u.Password, &u.Balance, &u.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return u, nil
}

func (s *Store) UpdateUser(ctx context.Context, u *models.User) error {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	query := `UPDATE users SET name=$1, email=$2, password=$3, balance=$4 WHERE id=$5`
	res, err := s.DB.ExecContext(ctx, query, u.Name, u.Email, u.Password, u.Balance, u.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("no user updated")
	}
	return nil
}

func (s *Store) DeleteUser(ctx context.Context, id int) error {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	_, err := s.DB.ExecContext(ctx, `DELETE FROM users WHERE id=$1`, id)
	return err
}

func (s *Store) ListUsers(ctx context.Context) ([]models.User, error) {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, email, balance, created_at FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.Balance, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, nil
}

// --- Books ---
func (s *Store) CreateBook(ctx context.Context, b *models.Book) error {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	if b.CreatedAt.IsZero() {
		b.CreatedAt = time.Now()
	}

	query := `INSERT INTO books (title, author_name, is_available, rent_price, created_at) VALUES ($1,$2,$3,$4,$5) RETURNING id`
	return s.DB.QueryRowContext(ctx, query, b.Title, b.AuthorName, b.IsAvailable, b.RentPrice, b.CreatedAt).Scan(&b.ID)
}

func (s *Store) GetBookByID(ctx context.Context, id int) (*models.Book, error) {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	b := &models.Book{}
	query := `SELECT id, title, author_name, is_available, rent_price, created_at FROM books WHERE id=$1`
	row := s.DB.QueryRowContext(ctx, query, id)
	if err := row.Scan(&b.ID, &b.Title, &b.AuthorName, &b.IsAvailable, &b.RentPrice, &b.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return b, nil
}

func (s *Store) UpdateBook(ctx context.Context, b *models.Book) error {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	query := `UPDATE books SET title=$1, author_name=$2, is_available=$3, rent_price=$4 WHERE id=$5`
	res, err := s.DB.ExecContext(ctx, query, b.Title, b.AuthorName, b.IsAvailable, b.RentPrice, b.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("no book updated")
	}
	return nil
}

func (s *Store) DeleteBook(ctx context.Context, id int) error {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	_, err := s.DB.ExecContext(ctx, `DELETE FROM books WHERE id=$1`, id)
	return err
}

func (s *Store) ListBooks(ctx context.Context) ([]models.Book, error) {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	rows, err := s.DB.QueryContext(ctx, `SELECT id, title, author_name, is_available, rent_price, created_at FROM books ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Book
	for rows.Next() {
		var b models.Book
		if err := rows.Scan(&b.ID, &b.Title, &b.AuthorName, &b.IsAvailable, &b.RentPrice, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

// --- Borrow Records ---
func (s *Store) CreateBorrowRecord(ctx context.Context, r *models.BorrowRecord) error {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	if r.BorrowedAt.IsZero() {
		r.BorrowedAt = time.Now()
	}

	query := `INSERT INTO borrow_records (user_id, book_id, borrowed_at, due_date, returned_at, rent_paid) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`
	var returnedAt interface{}
	if r.ReturnedAt != nil {
		returnedAt = *r.ReturnedAt
	} else {
		returnedAt = nil
	}
	return s.DB.QueryRowContext(ctx, query, r.UserID, r.BookID, r.BorrowedAt, r.DueDate, returnedAt, r.RentPaid).Scan(&r.ID)
}

func (s *Store) GetBorrowRecordByID(ctx context.Context, id int) (*models.BorrowRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	r := &models.BorrowRecord{}
	var returnedAt sql.NullTime
	query := `SELECT id, user_id, book_id, borrowed_at, due_date, returned_at, rent_paid FROM borrow_records WHERE id=$1`
	row := s.DB.QueryRowContext(ctx, query, id)
	if err := row.Scan(&r.ID, &r.UserID, &r.BookID, &r.BorrowedAt, &r.DueDate, &returnedAt, &r.RentPaid); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if returnedAt.Valid {
		t := returnedAt.Time
		r.ReturnedAt = &t
	} else {
		r.ReturnedAt = nil
	}
	return r, nil
}

func (s *Store) UpdateBorrowRecord(ctx context.Context, r *models.BorrowRecord) error {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	var returnedAt interface{}
	if r.ReturnedAt != nil {
		returnedAt = *r.ReturnedAt
	} else {
		returnedAt = nil
	}

	query := `UPDATE borrow_records SET user_id=$1, book_id=$2, borrowed_at=$3, due_date=$4, returned_at=$5, rent_paid=$6 WHERE id=$7`
	res, err := s.DB.ExecContext(ctx, query, r.UserID, r.BookID, r.BorrowedAt, r.DueDate, returnedAt, r.RentPaid, r.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("no borrow record updated")
	}
	return nil
}

func (s *Store) DeleteBorrowRecord(ctx context.Context, id int) error {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	_, err := s.DB.ExecContext(ctx, `DELETE FROM borrow_records WHERE id=$1`, id)
	return err
}

func (s *Store) ListBorrowRecordsByUser(ctx context.Context, userID int) ([]models.BorrowRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	rows, err := s.DB.QueryContext(ctx, `SELECT id, user_id, book_id, borrowed_at, due_date, returned_at, rent_paid FROM borrow_records WHERE user_id=$1 ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.BorrowRecord
	for rows.Next() {
		var r models.BorrowRecord
		var returnedAt sql.NullTime
		if err := rows.Scan(&r.ID, &r.UserID, &r.BookID, &r.BorrowedAt, &r.DueDate, &returnedAt, &r.RentPaid); err != nil {
			return nil, err
		}
		if returnedAt.Valid {
			t := returnedAt.Time
			r.ReturnedAt = &t
		} else {
			r.ReturnedAt = nil
		}
		out = append(out, r)
	}
	return out, nil
}
