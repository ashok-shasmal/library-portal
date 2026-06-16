package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/ashok-shasmal/library-portal/internal/pb"
	"github.com/gorilla/mux"
)

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

//         handler = RequireRole("ADMIN", handler)
//         handler = Authenticate(s.Store)(handler)

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
		adminOnly := Authenticate(s.Store)(RequireRole("ADMIN", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		h := &AuthHandler{Store: s.Store, TokenExpiry: 24 * time.Hour}
		h.Register(w, r)
	default:
		log.Printf("usersHandler method not allowed: %s", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) userByIDHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("userByIDHandler start: %s %s", r.Method, r.URL.Path)
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		log.Printf("userByIDHandler parse error: %v", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	authenticated := Authenticate(s.Store)
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
		authenticated(RequireRole("ADMIN", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		if books, hit, err := s.getCachedBooks(r.Context()); err != nil {
			log.Printf("booksHandler GET cache error: %v", err)
		} else if hit {
			log.Printf("booksHandler GET returned %d books from cache", len(books))
			writeJSON(w, books)
			return
		}

		books, err := s.Store.ListBooks(r.Context())
		if err != nil {
			log.Printf("booksHandler GET ListBooks error: %v", err)
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if err := s.setCachedBooks(r.Context(), books); err != nil {
			log.Printf("booksHandler GET cache set error: %v", err)
		}
		log.Printf("booksHandler GET returning %d books", len(books))
		writeJSON(w, books)
	case http.MethodPost:
		log.Printf("booksHandler POST request")
		adminOnly := Authenticate(s.Store)(RequireRole("ADMIN", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			if err := s.invalidateBookCache(r.Context(), int(b.Id)); err != nil {
				log.Printf("booksHandler POST cache invalidation error: %v", err)
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
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		log.Printf("bookByIDHandler parse error: %v", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if b, hit, err := s.getCachedBook(r.Context(), id); err != nil {
			log.Printf("bookByIDHandler GET cache error: %v", err)
		} else if hit {
			log.Printf("bookByIDHandler GET returned book id=%d from cache", id)
			writeJSON(w, b)
			return
		}
		b, err := s.Store.GetBookByID(r.Context(), id)
		if err != nil {
			http.Error(w, "server error", http.StatusInternalServerError)
			return
		}
		if b == nil {
			http.NotFound(w, r)
			return
		}
		if err := s.setCachedBook(r.Context(), id, b); err != nil {
			log.Printf("bookByIDHandler GET cache set error: %v", err)
		}
		writeJSON(w, b)
	case http.MethodPut:
		adminOnly := Authenticate(s.Store)(RequireRole("ADMIN", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			if err := s.invalidateBookCache(r.Context(), id); err != nil {
				log.Printf("bookByIDHandler PUT cache invalidation error: %v", err)
			}
			writeJSON(w, map[string]string{"status": "ok"})
		})))
		adminOnly.ServeHTTP(w, r)
	case http.MethodDelete:
		adminOnly := Authenticate(s.Store)(RequireRole("ADMIN", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := s.Store.DeleteBook(r.Context(), id); err != nil {
				http.Error(w, "server error", http.StatusInternalServerError)
				return
			}
			if err := s.invalidateBookCache(r.Context(), id); err != nil {
				log.Printf("bookByIDHandler DELETE cache invalidation error: %v", err)
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
		Authenticate(s.Store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		s.ensureBorrowWorkers()
		job := borrowJob{
			w:    w,
			r:    r,
			done: make(chan struct{}),
		}
		select {
		case s.borrowJobQueue <- job:
			<-job.done
			return
		default:
			log.Printf("borrowRecordsHandler POST rejected: worker pool full")
			http.Error(w, "service overloaded, too many borrow requests", http.StatusTooManyRequests)
			return
		}
	default:
		log.Printf("borrowRecordsHandler method not allowed: %s", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) borrowRecordByIDHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	authenticated := Authenticate(s.Store)
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
		adminOnly := authenticated(RequireRole("ADMIN", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		adminOnly := authenticated(RequireRole("ADMIN", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
