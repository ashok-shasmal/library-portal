package server

// server.go contains the HTTP server and background job orchestration
// for the library portal, including auth, book handling, and payment integration.

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ashok-shasmal/library-portal/internal/database"
	"github.com/ashok-shasmal/library-portal/internal/pb"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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
	authH := &AuthHandler{Store: s.Store, TokenExpiry: 24 * time.Hour}
	router.HandleFunc("/register", authH.Register).Methods(http.MethodPost)
	router.HandleFunc("/login", authH.Login).Methods(http.MethodPost)

	// Users
	router.HandleFunc("/users", s.usersHandler).Methods(http.MethodGet, http.MethodPost)
	router.HandleFunc("/users/{id:[0-9]+}", s.userByIDHandler).Methods(http.MethodGet, http.MethodPut, http.MethodDelete)

	// Books
	router.HandleFunc("/books", s.booksHandler).Methods(http.MethodGet, http.MethodPost)
	router.HandleFunc("/books/{id:[0-9]+}", s.bookByIDHandler).Methods(http.MethodGet, http.MethodPut, http.MethodDelete)

	// Borrow records
	borrowRecordsHandler := Authenticate(s.Store)(http.HandlerFunc(s.borrowRecordsHandler))
	router.Handle("/borrow_records", borrowRecordsHandler).Methods(http.MethodGet, http.MethodPost)
	router.Handle("/borrow_records/{id:[0-9]+}", Authenticate(s.Store)(http.HandlerFunc(s.borrowRecordByIDHandler))).Methods(http.MethodGet, http.MethodPut, http.MethodDelete)

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
