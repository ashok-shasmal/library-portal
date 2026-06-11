package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/ashok-shasmal/library-portal/internal/auth"
	"github.com/ashok-shasmal/library-portal/internal/database"
	"github.com/ashok-shasmal/library-portal/internal/models"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	Store       *database.Store
	TokenExpiry time.Duration
}

type registerReq struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
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

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "server error", http.StatusInternalServerError)
		return
	}

	u := &models.User{
		Name:      req.Name,
		Email:     req.Email,
		Password:  string(hashed),
		CreatedAt: time.Now(),
	}

	if err := h.Store.CreateUser(context.Background(), u); err != nil {
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	token, err := auth.GenerateToken(u.ID, h.TokenExpiry)
	if err != nil {
		http.Error(w, "could not generate token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(authResp{ID: u.ID, Name: u.Name, Email: u.Email, Token: token})
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

	token, err := auth.GenerateToken(u.ID, h.TokenExpiry)
	if err != nil {
		http.Error(w, "could not generate token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(authResp{ID: u.ID, Name: u.Name, Email: u.Email, Token: token})
}
