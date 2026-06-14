package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ashok-shasmal/library-portal/internal/auth"
	"github.com/ashok-shasmal/library-portal/internal/database"
	"github.com/ashok-shasmal/library-portal/internal/handlers"
	"github.com/ashok-shasmal/library-portal/internal/pb"
)

type Server struct {
	Store *database.Store
	Addr  string
	srv   *http.Server
}

var (
	isReady atomic.Bool
	isAlive atomic.Bool
)

func New(store *database.Store, addr string) *Server {
	return &Server{Store: store, Addr: addr}
}

func (s *Server) ListenAndServe() error {

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM)

	mux := http.NewServeMux()

	//Welcome Message
	mux.HandleFunc("/", s.welcome)

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

	// Readiness Probe
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		if !isReady.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	http.HandleFunc("/live", func(w http.ResponseWriter, r *http.Request) {
		if !isAlive.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	go func() {
		<-sig

		log.Println("Handling Signal SIGTERM")
		// Mark NOT READY so probes stop sending traffic
		isReady.Store(false)

		// allow in-flight requests to drain
		time.Sleep(10 * time.Second)

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := s.srv.Shutdown(ctx); err != nil {
			log.Printf("server shutdown failed: %v", err)
		}
	}()

	s.srv = &http.Server{Addr: s.Addr, Handler: mux}
	isReady.Store(true)
	isAlive.Store(true)
	log.Printf("server listening %s", s.Addr)
	if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
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

func scrubUserPassword(u *pb.User) {
	if u != nil {
		u.Password = ""
	}
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !auth.IsAdmin(r.Context()) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}

func (s *Server) requireSelfOrAdmin(w http.ResponseWriter, r *http.Request, targetID int) bool {
	uid, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	if uid != targetID && !auth.IsAdmin(r.Context()) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}

// -- Welcome ---
func (s *Server) welcome(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, "!!! Welocome to my Library !!! ")
}

// // --- Users handlers ---

// func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
//     users, err := s.Store.ListUsers(r.Context())
//     if err != nil {
//         log.Printf("usersHandler GET ListUsers error: %v", err)
//         http.Error(w, "server error", http.StatusInternalServerError)
//         return
//     }

//     for i := range users {
//         scrubUserPassword(&users[i])
//     }

//     log.Printf("usersHandler GET returning %d users", len(users))
//     writeJSON(w, users)
// }

// func (s *Server) usersHandler(w http.ResponseWriter, r *http.Request) {
// 	log.Printf("usersHandler start: %s %s", r.Method, r.URL.Path)
// 	switch r.Method {
// 	case http.MethodGet:
// 		log.Printf("usersHandler GET request")

// 		var handler http.Handler = http.HandlerFunc(s.handleListUsers)

//         handler = auth.RequireRole("ADMIN", handler)
//         handler = auth.Authenticate(s.Store)(handler)

//         handler.ServeHTTP(w, r)

// 	case http.MethodPost:
// 		log.Printf("usersHandler POST delegate register")
// 		// create user (registration already exists) - delegate to handler
// 		h := &handlers.AuthHandler{Store: s.Store, TokenExpiry: 24 * time.Hour}
// 		h.Register(w, r)
// 	default:
// 		log.Printf("usersHandler method not allowed: %s", r.Method)
// 		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
// 	}
// }

// --- Users handlers ---
// Nested way of doing the same thing
func (s *Server) usersHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("usersHandler start: %s %s", r.Method, r.URL.Path)
	switch r.Method {
	case http.MethodGet:
		log.Printf("usersHandler GET request")
		adminOnly := auth.Authenticate(s.Store)(auth.RequireRole("ADMIN", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			users, err := s.Store.ListUsers(r.Context())
			if err != nil {
				log.Printf("usersHandler GET ListUsers error: %v", err)
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			for i := range users {
				scrubUserPassword(&users[i])
			}
			log.Printf("usersHandler GET returning %d users", len(users))
			writeJSON(w, users)
		})))
		adminOnly.ServeHTTP(w, r)
	case http.MethodPost:
		log.Printf("usersHandler POST delegate register")
		// create user (registration already exists) - delegate to handler
		h := &handlers.AuthHandler{Store: s.Store, TokenExpiry: 24 * time.Hour}
		h.Register(w, r)
	default:
		log.Printf("usersHandler method not allowed: %s", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) userByIDHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("userByIDHandler start: %s %s", r.Method, r.URL.Path)
	id, err := parseIDFromPath("/users/", r.URL.Path)
	if err != nil {
		log.Printf("userByIDHandler parse error: %v", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	authenticated := auth.Authenticate(s.Store)
	switch r.Method {
	case http.MethodGet:
		log.Printf("userByIDHandler GET id=%d", id)
		authenticated(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !s.requireSelfOrAdmin(w, r, id) {
				return
			}
			u, err := s.Store.GetUserByID(r.Context(), id)
			if err != nil {
				log.Printf("userByIDHandler GET GetUserByID error id=%d: %v", id, err)
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			if u == nil {
				log.Printf("userByIDHandler GET not found id=%d", id)
				http.NotFound(w, r)
				return
			}
			scrubUserPassword(u)
			writeJSON(w, u)
		})).ServeHTTP(w, r)
	case http.MethodPut:
		log.Printf("userByIDHandler PUT id=%d", id)
		authenticated(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !s.requireSelfOrAdmin(w, r, id) {
				return
			}
			var u pb.User
			if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
				log.Printf("userByIDHandler PUT decode error: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			u.Id = int32(id)
			if err := s.Store.UpdateUser(r.Context(), &u); err != nil {
				log.Printf("userByIDHandler PUT UpdateUser error id=%d: %v", id, err)
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]string{"status": "ok"})
		})).ServeHTTP(w, r)
	case http.MethodDelete:
		log.Printf("userByIDHandler DELETE id=%d", id)
		authenticated(auth.RequireRole("ADMIN", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := s.Store.DeleteUser(r.Context(), id); err != nil {
				log.Printf("userByIDHandler DELETE DeleteUser error id=%d: %v", id, err)
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]string{"status": "deleted"})
		}))).ServeHTTP(w, r)
	default:
		log.Printf("userByIDHandler method not allowed: %s", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- Books handlers ---
func (s *Server) booksHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("booksHandler start: %s %s", r.Method, r.URL.Path)
	switch r.Method {
	case http.MethodGet:
		log.Printf("booksHandler GET request")
		books, err := s.Store.ListBooks(r.Context())
		if err != nil {
			log.Printf("booksHandler GET ListBooks error: %v", err)
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		log.Printf("booksHandler GET returning %d books", len(books))
		writeJSON(w, books)
	case http.MethodPost:
		log.Printf("booksHandler POST request")
		adminOnly := auth.Authenticate(s.Store)(auth.RequireRole("ADMIN", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var b pb.Book
			if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
				log.Printf("booksHandler POST decode error: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			b.IsAvailable = true
			if err := s.Store.CreateBook(r.Context(), &b); err != nil {
				log.Printf("booksHandler POST CreateBook error: %v", err)
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			log.Printf("booksHandler POST created book id=%d title=%s", b.Id, b.Title)
			writeJSON(w, b)
		})))
		adminOnly.ServeHTTP(w, r)
	default:
		log.Printf("booksHandler method not allowed: %s", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) bookByIDHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("bookByIDHandler start: %s %s", r.Method, r.URL.Path)
	id, err := parseIDFromPath("/books/", r.URL.Path)
	if err != nil {
		log.Printf("bookByIDHandler parse error: %v", err)
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
		adminOnly := auth.Authenticate(s.Store)(auth.RequireRole("ADMIN", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var b pb.Book
			if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			b.Id = int32(id)
			if err := s.Store.UpdateBook(r.Context(), &b); err != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]string{"status": "ok"})
		})))
		adminOnly.ServeHTTP(w, r)
	case http.MethodDelete:
		adminOnly := auth.Authenticate(s.Store)(auth.RequireRole("ADMIN", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := s.Store.DeleteBook(r.Context(), id); err != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]string{"status": "deleted"})
		})))
		adminOnly.ServeHTTP(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// --- Borrow Records handlers ---
func (s *Server) borrowRecordsHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("borrowRecordsHandler start: %s %s", r.Method, r.URL.Path)
	switch r.Method {
	case http.MethodGet:
		log.Printf("borrowRecordsHandler GET request")
		q := r.URL.Query().Get("user_id")
		if q == "" {
			log.Printf("borrowRecordsHandler GET missing user_id")
			http.Error(w, "user_id required", http.StatusBadRequest)
			return
		}
		uid, err := strconv.Atoi(q)
		if err != nil {
			log.Printf("borrowRecordsHandler GET bad user_id: %v", err)
			http.Error(w, "bad user_id", http.StatusBadRequest)
			return
		}
		auth.Authenticate(s.Store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !s.requireSelfOrAdmin(w, r, uid) {
				return
			}
			records, err := s.Store.ListBorrowRecordsByUser(r.Context(), uid)
			if err != nil {
				log.Printf("borrowRecordsHandler GET ListBorrowRecordsByUser error uid=%d: %v", uid, err)
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			log.Printf("borrowRecordsHandler GET returning %d records for user_id=%d", len(records), uid)
			writeJSON(w, records)
		})).ServeHTTP(w, r)
	case http.MethodPost:
		log.Printf("borrowRecordsHandler POST request")
		auth.Authenticate(s.Store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var rec pb.BorrowRecord
			if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
				log.Printf("borrowRecordsHandler POST decode error: %v", err)
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			uid, ok := auth.UserIDFromContext(r.Context())
			if !ok {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if rec.UserId == 0 {
				rec.UserId = int32(uid)
			}
			if !auth.IsAdmin(r.Context()) && int(rec.UserId) != uid {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}

			book, err := s.Store.GetBookByID(r.Context(), int(rec.BookId))
			if err != nil {
				log.Printf("borrowRecordsHandler POST GetBookByID error: %v", err)
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			if book == nil {
				http.Error(w, "book not found", http.StatusNotFound)
				return
			}
			if !book.IsAvailable {
				http.Error(w, "book unavailable", http.StatusBadRequest)
				return
			}
			if err := s.Store.CreateBorrowRecord(r.Context(), &rec); err != nil {
				log.Printf("borrowRecordsHandler POST CreateBorrowRecord error: %v", err)
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			log.Printf("borrowRecordsHandler POST created record id=%d user_id=%d book_id=%d", rec.Id, rec.UserId, rec.BookId)
			writeJSON(w, rec)
		})).ServeHTTP(w, r)
	default:
		log.Printf("borrowRecordsHandler method not allowed: %s", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) borrowRecordByIDHandler(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDFromPath("/borrow_records/", r.URL.Path)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	authenticated := auth.Authenticate(s.Store)
	switch r.Method {
	case http.MethodGet:
		authenticated(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec, err := s.Store.GetBorrowRecordByID(r.Context(), id)
			if err != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			if rec == nil {
				http.NotFound(w, r)
				return
			}
			if !s.requireSelfOrAdmin(w, r, int(rec.UserId)) {
				return
			}
			writeJSON(w, rec)
		})).ServeHTTP(w, r)
	case http.MethodPut:
		adminOnly := authenticated(auth.RequireRole("ADMIN", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var rec pb.BorrowRecord
			if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
				http.Error(w, "bad request", http.StatusBadRequest)
				return
			}
			rec.Id = int32(id)
			if err := s.Store.UpdateBorrowRecord(r.Context(), &rec); err != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]string{"status": "ok"})
		})))
		adminOnly.ServeHTTP(w, r)
	case http.MethodDelete:
		adminOnly := authenticated(auth.RequireRole("ADMIN", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := s.Store.DeleteBorrowRecord(r.Context(), id); err != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]string{"status": "deleted"})
		})))
		adminOnly.ServeHTTP(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
