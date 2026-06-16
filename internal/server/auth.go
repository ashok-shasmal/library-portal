package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ashok-shasmal/library-portal/internal/database"
	"github.com/ashok-shasmal/library-portal/internal/pb"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type AuthHandler struct {
	Store       *database.Store
	TokenExpiry time.Duration
}

type registerReq struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role,omitempty"`
}

type authResp struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Token string `json:"token"`
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	role := strings.ToUpper(strings.TrimSpace(req.Role))
	if role == "" {
		role = "USER"
	}
	if role != "USER" && role != "ADMIN" {
		http.Error(w, "invalid role", http.StatusBadRequest)
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	u := &pb.User{
		Name:      req.Name,
		Email:     req.Email,
		Password:  string(hashed),
		Role:      role,
		CreatedAt: timestamppb.New(time.Now()),
	}

	if err := h.Store.CreateUser(context.Background(), u); err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	token, err := GenerateToken(int(u.Id), h.TokenExpiry)
	if err != nil {
		http.Error(w, "could not generate token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(authResp{ID: int(u.Id), Name: u.Name, Email: u.Email, Token: token})
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	u, err := h.Store.GetUserByEmail(context.Background(), req.Email)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}
	if u == nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(req.Password)); err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	token, err := GenerateToken(int(u.Id), h.TokenExpiry)
	if err != nil {
		http.Error(w, "could not generate token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(authResp{ID: int(u.Id), Name: u.Name, Email: u.Email, Token: token})
}
