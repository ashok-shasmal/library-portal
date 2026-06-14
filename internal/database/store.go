package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/ashok-shasmal/library-portal/internal/pb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Store struct {
	DB      *sql.DB
	Timeout time.Duration
}

func NewStore(db *sql.DB) *Store {
	return &Store{DB: db, Timeout: 5 * time.Second}
}

// --- Users ---
func (s *Store) CreateUser(ctx context.Context, u *pb.User) error {
	log.Printf("Creating user: %s (%s)", u.Name, u.Email) // Debug log
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	if u.GetCreatedAt() == nil || u.GetCreatedAt().AsTime().IsZero() {
		u.CreatedAt = timestamppb.New(time.Now())
	}

	createdAt := u.GetCreatedAt().AsTime()
	if createdAt.IsZero() {
		createdAt = time.Now()
		u.CreatedAt = timestamppb.New(createdAt)
	}

	if u.Role == "" {
		u.Role = "USER"
	}
	query := `INSERT INTO users (name, email, password, balance, role, created_at) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`
	if err := s.DB.QueryRowContext(ctx, query, u.Name, u.Email, u.Password, u.Balance, u.Role, createdAt).Scan(&u.Id); err != nil {
		log.Printf("Error creating user: %v", err.Error()) // Debug log
		return err
	}
	log.Printf("User created with ID: %d", u.Id) // Debug log
	return nil
}

func (s *Store) GetUserByID(ctx context.Context, id int) (*pb.User, error) {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	u := &pb.User{}
	var createdAt time.Time
	query := `SELECT id, name, email, password, balance, role, created_at FROM users WHERE id = $1`
	row := s.DB.QueryRowContext(ctx, query, id)
	if err := row.Scan(&u.Id, &u.Name, &u.Email, &u.Password, &u.Balance, &u.Role, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	u.CreatedAt = timestamppb.New(createdAt)
	return u, nil
}

func (s *Store) GetUserByEmail(ctx context.Context, email string) (*pb.User, error) {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	u := &pb.User{}
	var createdAt time.Time
	query := `SELECT id, name, email, password, balance, role, created_at FROM users WHERE email = $1`
	row := s.DB.QueryRowContext(ctx, query, email)
	if err := row.Scan(&u.Id, &u.Name, &u.Email, &u.Password, &u.Balance, &u.Role, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	u.CreatedAt = timestamppb.New(createdAt)
	return u, nil
}

func (s *Store) UpdateUser(ctx context.Context, u *pb.User) error {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	query := `UPDATE users SET name=$1, email=$2, password=$3, balance=$4, role=$5 WHERE id=$6`
	res, err := s.DB.ExecContext(ctx, query, u.Name, u.Email, u.Password, u.Balance, u.Role, u.Id)
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

func (s *Store) ListUsers(ctx context.Context) ([]pb.User, error) {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, email, balance, role, created_at FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []pb.User
	for rows.Next() {
		var u pb.User
		var createdAt time.Time
		if err := rows.Scan(&u.Id, &u.Name, &u.Email, &u.Balance, &u.Role, &createdAt); err != nil {
			return nil, err
		}
		u.CreatedAt = timestamppb.New(createdAt)
		out = append(out, u)
	}
	return out, nil
}

// --- Books ---
func (s *Store) CreateBook(ctx context.Context, b *pb.Book) error {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	createdAt := b.GetCreatedAt().AsTime()
	if createdAt.IsZero() {
		createdAt = time.Now()
		b.CreatedAt = timestamppb.New(createdAt)
	}

	query := `INSERT INTO books (title, author_name, is_available, rent_price, created_at) VALUES ($1,$2,$3,$4,$5) RETURNING id`
	return s.DB.QueryRowContext(ctx, query, b.Title, b.AuthorName, b.IsAvailable, b.RentPrice, createdAt).Scan(&b.Id)
}

func (s *Store) GetBookByID(ctx context.Context, id int) (*pb.Book, error) {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	b := &pb.Book{}
	var createdAt time.Time
	query := `SELECT id, title, author_name, is_available, rent_price, created_at FROM books WHERE id=$1`
	row := s.DB.QueryRowContext(ctx, query, id)
	if err := row.Scan(&b.Id, &b.Title, &b.AuthorName, &b.IsAvailable, &b.RentPrice, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	b.CreatedAt = timestamppb.New(createdAt)
	return b, nil
}

func (s *Store) UpdateBook(ctx context.Context, b *pb.Book) error {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	query := `UPDATE books SET title=$1, author_name=$2, is_available=$3, rent_price=$4 WHERE id=$5`
	res, err := s.DB.ExecContext(ctx, query, b.Title, b.AuthorName, b.IsAvailable, b.RentPrice, b.Id)
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

func (s *Store) ListBooks(ctx context.Context) ([]pb.Book, error) {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	rows, err := s.DB.QueryContext(ctx, `SELECT id, title, author_name, is_available, rent_price, created_at FROM books ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []pb.Book
	for rows.Next() {
		var b pb.Book
		var createdAt time.Time
		if err := rows.Scan(&b.Id, &b.Title, &b.AuthorName, &b.IsAvailable, &b.RentPrice, &createdAt); err != nil {
			return nil, err
		}
		b.CreatedAt = timestamppb.New(createdAt)
		out = append(out, b)
	}
	return out, nil
}

// --- Borrow Records ---
func (s *Store) CreateBorrowRecord(ctx context.Context, r *pb.BorrowRecord) error {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	borrowedAt := r.GetBorrowedAt().AsTime()
	if borrowedAt.IsZero() {
		borrowedAt = time.Now()
		r.BorrowedAt = timestamppb.New(borrowedAt)
	}
	dueDate := r.GetDueDate().AsTime()
	var returnedAt interface{}
	if r.ReturnedAt != nil {
		returnedAt = r.ReturnedAt.AsTime()
	} else {
		returnedAt = nil
	}

	var err error
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	var isAvailable bool
	if err = tx.QueryRowContext(ctx, `SELECT is_available FROM books WHERE id=$1 FOR UPDATE`, r.BookId).Scan(&isAvailable); err != nil {
		return err
	}
	if !isAvailable {
		return fmt.Errorf("book unavailable")
	}

	insertQuery := `INSERT INTO borrow_records (user_id, book_id, borrowed_at, due_date, returned_at, rent_paid) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`
	if err = tx.QueryRowContext(ctx, insertQuery, r.UserId, r.BookId, borrowedAt, dueDate, returnedAt, r.RentPaid).Scan(&r.Id); err != nil {
		return err
	}

	if _, err = tx.ExecContext(ctx, `UPDATE books SET is_available=false WHERE id=$1`, r.BookId); err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *Store) GetBorrowRecordByID(ctx context.Context, id int) (*pb.BorrowRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	r := &pb.BorrowRecord{}
	var borrowedAt time.Time
	var dueDate time.Time
	var returnedAt sql.NullTime
	query := `SELECT id, user_id, book_id, borrowed_at, due_date, returned_at, rent_paid FROM borrow_records WHERE id=$1`
	row := s.DB.QueryRowContext(ctx, query, id)
	if err := row.Scan(&r.Id, &r.UserId, &r.BookId, &borrowedAt, &dueDate, &returnedAt, &r.RentPaid); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	r.BorrowedAt = timestamppb.New(borrowedAt)
	r.DueDate = timestamppb.New(dueDate)
	if returnedAt.Valid {
		r.ReturnedAt = timestamppb.New(returnedAt.Time)
	} else {
		r.ReturnedAt = nil
	}
	return r, nil
}

func (s *Store) UpdateBorrowRecord(ctx context.Context, r *pb.BorrowRecord) error {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	borrowedAt := r.GetBorrowedAt().AsTime()
	dueDate := r.GetDueDate().AsTime()
	var returnedAt interface{}
	if r.ReturnedAt != nil {
		returnedAt = r.ReturnedAt.AsTime()
	} else {
		returnedAt = nil
	}

	query := `UPDATE borrow_records SET user_id=$1, book_id=$2, borrowed_at=$3, due_date=$4, returned_at=$5, rent_paid=$6 WHERE id=$7`
	res, err := s.DB.ExecContext(ctx, query, r.UserId, r.BookId, borrowedAt, dueDate, returnedAt, r.RentPaid, r.Id)
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

func (s *Store) ListBorrowRecordsByUser(ctx context.Context, userID int) ([]pb.BorrowRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	rows, err := s.DB.QueryContext(ctx, `SELECT id, user_id, book_id, borrowed_at, due_date, returned_at, rent_paid FROM borrow_records WHERE user_id=$1 ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []pb.BorrowRecord
	for rows.Next() {
		var r pb.BorrowRecord
		var borrowedAt time.Time
		var dueDate time.Time
		var returnedAt sql.NullTime
		if err := rows.Scan(&r.Id, &r.UserId, &r.BookId, &borrowedAt, &dueDate, &returnedAt, &r.RentPaid); err != nil {
			return nil, err
		}
		r.BorrowedAt = timestamppb.New(borrowedAt)
		r.DueDate = timestamppb.New(dueDate)
		if returnedAt.Valid {
			r.ReturnedAt = timestamppb.New(returnedAt.Time)
		} else {
			r.ReturnedAt = nil
		}
		out = append(out, r)
	}
	return out, nil
}
