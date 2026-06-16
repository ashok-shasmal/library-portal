package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/ashok-shasmal/library-portal/internal/pb"
	"github.com/go-redis/redis/v8"
	"google.golang.org/protobuf/types/known/timestamppb"
)

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
	if !IsAdmin(r.Context()) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}

func (s *Server) requireSelfOrAdmin(w http.ResponseWriter, r *http.Request, targetID int) bool {
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	if uid != targetID && !IsAdmin(r.Context()) {
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
	uid, ok := UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if rec.UserId == 0 {
		rec.UserId = int32(uid)
	}
	if !IsAdmin(r.Context()) && int(rec.UserId) != uid {
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
