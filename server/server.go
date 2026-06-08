package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ashok-shasmal/library-portal/internal/auth"
	"github.com/ashok-shasmal/library-portal/internal/database"
	"github.com/ashok-shasmal/library-portal/internal/handlers"
	"github.com/ashok-shasmal/library-portal/internal/models"
)

type Server struct {
	Store *database.Store
	Addr  string
	srv   *http.Server
}

func New(store *database.Store, addr string) *Server {
	return &Server{Store: store, Addr: addr}
}

func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()

	// Auth handlers
	authH := &handlers.AuthHandler{Store: s.Store, TokenExpiry: 24 * time.Hour}
	mux.HandleFunc("/register", authH.Register)
	mux.HandleFunc("/login", authH.Login)

	// Users
	mux.HandleFunc("/users", s.usersHandler)
	mux.HandleFunc("/users/", s.userByIDHandler)

	// Books
	mux.HandleFunc("/books", s.booksHandler)
	mux.HandleFunc("/books/", s.bookByIDHandler)

	// Borrow records
	mux.HandleFunc("/borrow_records", s.borrowRecordsHandler)
	mux.HandleFunc("/borrow_records/", s.borrowRecordByIDHandler)

	s.srv = &http.Server{Addr: s.Addr, Handler: mux}
	log.Printf("server listening %s", s.Addr)
	return s.srv.ListenAndServe()
}

// --- Helpers ---
func parseIDFromPath(prefix, path string) (int, error) {
	idStr := strings.TrimPrefix(path, prefix)
	idStr = strings.Trim(idStr, "/")
	if idStr == "" {
		return 0, fmt.Errorf("missing id")
	}
	return strconv.Atoi(idStr)
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// --- Users handlers ---
func (s *Server) usersHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// protected
		auth.JWTMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			users, err := s.Store.ListUsers(r.Context())
			if err != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, users)
		})).ServeHTTP(w, r)
	case http.MethodPost:
		// create user (registration already exists) - delegate to handler
		h := &handlers.AuthHandler{Store: s.Store, TokenExpiry: 24 * time.Hour}
		h.Register(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) userByIDHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDFromPath("/users/", r.URL.Path)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		auth.JWTMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, err := s.Store.GetUserByID(r.Context(), id)
			if err != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			if u == nil {
				http.NotFound(w, r)
				return
			}
			writeJSON(w, u)
		})).ServeHTTP(w, r)
	case http.MethodPut:
		auth.JWTMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var u models.User
			if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			u.ID = id
			if err := s.Store.UpdateUser(r.Context(), &u); err != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]string{"status": "ok"})
		})).ServeHTTP(w, r)
	case http.MethodDelete:
		auth.JWTMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := s.Store.DeleteUser(r.Context(), id); err != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]string{"status": "deleted"})
		})).ServeHTTP(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- Books handlers ---
func (s *Server) booksHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// public listing allowed, but use auth if desired
		books, err := s.Store.ListBooks(r.Context())
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, books)
	case http.MethodPost:
		// create book - protected
		auth.JWTMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var b models.Book
			if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			if err := s.Store.CreateBook(r.Context(), &b); err != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, b)
		})).ServeHTTP(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) bookByIDHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDFromPath("/books/", r.URL.Path)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		b, err := s.Store.GetBookByID(r.Context(), id)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if b == nil {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, b)
	case http.MethodPut:
		auth.JWTMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var b models.Book
			if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			b.ID = id
			if err := s.Store.UpdateBook(r.Context(), &b); err != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]string{"status": "ok"})
		})).ServeHTTP(w, r)
	case http.MethodDelete:
		auth.JWTMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := s.Store.DeleteBook(r.Context(), id); err != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]string{"status": "deleted"})
		})).ServeHTTP(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- Borrow Records handlers ---
func (s *Server) borrowRecordsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// requires user id as query param: ?user_id=123
		q := r.URL.Query().Get("user_id")
		if q == "" {
			http.Error(w, "user_id required", http.StatusBadRequest)
			return
		}
		uid, err := strconv.Atoi(q)
		if err != nil {
			http.Error(w, "bad user_id", http.StatusBadRequest)
			return
		}
		// protected
		auth.JWTMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			records, err := s.Store.ListBorrowRecordsByUser(r.Context(), uid)
			if err != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, records)
		})).ServeHTTP(w, r)
	case http.MethodPost:
		// create borrow record - protected
		auth.JWTMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var rec models.BorrowRecord
			if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			if err := s.Store.CreateBorrowRecord(r.Context(), &rec); err != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, rec)
		})).ServeHTTP(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) borrowRecordByIDHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDFromPath("/borrow_records/", r.URL.Path)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		rec, err := s.Store.GetBorrowRecordByID(r.Context(), id)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if rec == nil {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, rec)
	case http.MethodPut:
		auth.JWTMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var rec models.BorrowRecord
			if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			rec.ID = id
			if err := s.Store.UpdateBorrowRecord(r.Context(), &rec); err != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]string{"status": "ok"})
		})).ServeHTTP(w, r)
	case http.MethodDelete:
		auth.JWTMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := s.Store.DeleteBorrowRecord(r.Context(), id); err != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]string{"status": "deleted"})
		})).ServeHTTP(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
