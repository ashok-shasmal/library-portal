package server

// server.go contains the HTTP server and background job orchestration
// for the library portal, including auth, book handling, and payment integration.

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ashok-shasmal/library-portal/internal/auth"
	"github.com/ashok-shasmal/library-portal/internal/database"
	"github.com/ashok-shasmal/library-portal/internal/handlers"
	"github.com/ashok-shasmal/library-portal/internal/pb"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type borrowJob struct {
	w    http.ResponseWriter
	r    *http.Request
	done chan struct{}
}

type Server struct {
	Store             *database.Store
	Addr              string
	srv               *http.Server
	borrowJobQueue    chan borrowJob
	borrowWorkersOnce sync.Once
	paymentConn       *grpc.ClientConn
	paymentClient     pb.PaymentServiceClient
	redisClient       *redis.Client
}

var (
	isReady atomic.Bool
	isAlive atomic.Bool
)

func New(store *database.Store, addr, paymentAddress, redisAddress string) *Server {
	if paymentAddress == "" {
		paymentAddress = "localhost:50051"
	}

	conn, err := grpc.Dial(paymentAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to connect to payment service: %v", err)
	}

	var redisClient *redis.Client
	if redisAddress != "" {
		redisClient = redis.NewClient(&redis.Options{Addr: redisAddress})
		if err := redisClient.Ping(context.Background()).Err(); err != nil {
			log.Printf("warning: redis unavailable at %s: %v; caching disabled", redisAddress, err)
			redisClient = nil
		} else {
			log.Printf("redis cache enabled at %s", redisAddress)
		}
	}

	return &Server{
		Store:          store,
		Addr:           addr,
		borrowJobQueue: make(chan borrowJob, 10),
		paymentConn:    conn,
		paymentClient:  pb.NewPaymentServiceClient(conn),
		redisClient:    redisClient,
	}
}

func (s *Server) ListenAndServe() error {

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM)

	router := mux.NewRouter()

	// Welcome Message
	router.HandleFunc("/", s.welcome).Methods(http.MethodGet)

	// Auth handlers
	authH := &handlers.AuthHandler{Store: s.Store, TokenExpiry: 24 * time.Hour}
	router.HandleFunc("/register", authH.Register).Methods(http.MethodPost)
	router.HandleFunc("/login", authH.Login).Methods(http.MethodPost)

	// Users
	router.HandleFunc("/users", s.usersHandler).Methods(http.MethodGet, http.MethodPost)
	router.HandleFunc("/users/{id:[0-9]+}", s.userByIDHandler).Methods(http.MethodGet, http.MethodPut, http.MethodDelete)

	// Books
	router.HandleFunc("/books", s.booksHandler).Methods(http.MethodGet, http.MethodPost)
	router.HandleFunc("/books/{id:[0-9]+}", s.bookByIDHandler).Methods(http.MethodGet, http.MethodPut, http.MethodDelete)

	// Borrow records
	borrowRecordsHandler := auth.Authenticate(s.Store)(http.HandlerFunc(s.borrowRecordsHandler))
	router.Handle("/borrow_records", borrowRecordsHandler).Methods(http.MethodGet, http.MethodPost)
	router.Handle("/borrow_records/{id:[0-9]+}", auth.Authenticate(s.Store)(http.HandlerFunc(s.borrowRecordByIDHandler))).Methods(http.MethodGet, http.MethodPut, http.MethodDelete)

	// Readiness Probe
	router.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		if !isReady.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}).Methods(http.MethodGet)

	router.HandleFunc("/live", func(w http.ResponseWriter, r *http.Request) {
		if !isAlive.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}).Methods(http.MethodGet)

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

	s.srv = &http.Server{Addr: s.Addr, Handler: router}
	isReady.Store(true)
	isAlive.Store(true)
	log.Printf("server listening %s", s.Addr)
	if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// --- Helpers ---
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

func (s *Server) cacheKeyBookList() string {
	return "books:list"
}

func (s *Server) cacheKeyBook(id int) string {
	return fmt.Sprintf("books:%d", id)
}

func (s *Server) getCachedBooks(ctx context.Context) ([]pb.Book, bool, error) {
	if s.redisClient == nil {
		return nil, false, nil
	}
	data, err := s.redisClient.Get(ctx, s.cacheKeyBookList()).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var books []pb.Book
	if err := json.Unmarshal(data, &books); err != nil {
		return nil, false, err
	}
	return books, true, nil
}

func (s *Server) setCachedBooks(ctx context.Context, books []pb.Book) error {
	if s.redisClient == nil {
		return nil
	}
	data, err := json.Marshal(books)
	if err != nil {
		return err
	}
	return s.redisClient.Set(ctx, s.cacheKeyBookList(), data, 5*time.Minute).Err()
}

func (s *Server) getCachedBook(ctx context.Context, id int) (*pb.Book, bool, error) {
	if s.redisClient == nil {
		return nil, false, nil
	}
	data, err := s.redisClient.Get(ctx, s.cacheKeyBook(id)).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var book pb.Book
	if err := json.Unmarshal(data, &book); err != nil {
		return nil, false, err
	}
	return &book, true, nil
}

func (s *Server) setCachedBook(ctx context.Context, id int, book *pb.Book) error {
	if s.redisClient == nil {
		return nil
	}
	data, err := json.Marshal(book)
	if err != nil {
		return err
	}
	return s.redisClient.Set(ctx, s.cacheKeyBook(id), data, 5*time.Minute).Err()
}

func (s *Server) invalidateBookCache(ctx context.Context, id int) error {
	if s.redisClient == nil {
		return nil
	}
	keys := []string{s.cacheKeyBookList()}
	if id > 0 {
		keys = append(keys, s.cacheKeyBook(id))
	}
	return s.redisClient.Del(ctx, keys...).Err()
}

func (s *Server) ensureBorrowWorkers() {
	s.borrowWorkersOnce.Do(func() {
		for i := 0; i < 10; i++ {
			go func() {
				for job := range s.borrowJobQueue {
					s.processBorrowJob(job.w, job.r)
					close(job.done)
				}
			}()
		}
	})
}

func (s *Server) processBorrowJob(w http.ResponseWriter, r *http.Request) {
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

	if rec.DueDate == nil || rec.DueDate.AsTime().IsZero() {
		rec.DueDate = timestamppb.New(time.Now().Add(7 * 24 * time.Hour))
	}
	rec.RentPaid = 50.0

	payReq := &pb.PaymentRequest{
		UserId: int32(uid),
		BookId: rec.BookId,
		Amount: 50.0,
		Weeks:  1,
	}
	payResp, err := s.paymentClient.Charge(r.Context(), payReq)
	if err != nil {
		log.Printf("borrowRecordsHandler POST payment error: %v", err)
		http.Error(w, "payment failed", http.StatusPaymentRequired)
		return
	}
	if payResp == nil || !payResp.Success {
		log.Printf("borrowRecordsHandler POST payment declined: %v", payResp)
		http.Error(w, "payment declined", http.StatusPaymentRequired)
		return
	}

	if err := s.Store.CreateBorrowRecord(r.Context(), &rec); err != nil {
		log.Printf("borrowRecordsHandler POST CreateBorrowRecord error: %v", err)
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	log.Printf("borrowRecordsHandler POST created record id=%d user_id=%d book_id=%d", rec.Id, rec.UserId, rec.BookId)
	writeJSON(w, rec)
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
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
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
			if err := s.invalidateBookCache(r.Context(), id); err != nil {
				log.Printf("bookByIDHandler PUT cache invalidation error: %v", err)
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
