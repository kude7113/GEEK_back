package handler

import (
	"GEEK_back/apiutils"
	mw "GEEK_back/middleware"
	"GEEK_back/store"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/mux"
	"net/http"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
)

const sessionDuration = 24 * time.Hour

type Handler struct {
	Store *store.Store
}

type errorResponse struct {
	Error string `json:"error"`
}

func NewHandler(s *store.Store) *Handler {
	return &Handler{Store: s}
}

// registerRequest - тело запроса регистрации пользователя
// Пример:
// {
// "email": "user@example.com",
// "username": "johndoe",
// "password": "secret",
// "confirm_password": "secret"
// }
type registerRequest struct {
	Email           string `json:"email"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirm_password"`
}

// Register создает нового пользователя
// @Summary Register new user
// @Description Create a new user (email, username, password). Returns created user on success.
// @Tags auth
// @Accept json
// @Produce json
// @Param register body registerRequest true "Register request"
// @Success 201 {object} store.User
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /register [post]
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var request registerRequest
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		apiutils.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if request.Email == "" {
		apiutils.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "no email provided"})
		return
	}
	if request.Password == "" {
		apiutils.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "no password provided"})
		return
	}
	if request.ConfirmPassword == "" {
		apiutils.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "no confirm password provided"})
		return
	}
	if request.Password != request.ConfirmPassword {
		apiutils.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "passwords do not match"})
		return
	}

	user, err := h.Store.CreateUser(request.Email, request.Password)
	if errors.Is(err, store.ErrUserExists) {
		apiutils.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "user already exists"})
		return
	}
	if err != nil {
		apiutils.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("error creating user: %s", err)})
		return
	}

	apiutils.WriteJSON(w, http.StatusCreated, user)
}

// LoginRequest - тело запроса для входа
// Пример:
// {
// "email": "user@example.com",
// "password": "secret"
// }
type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Login аутентифицирует пользователя и устанавливает cookie-сессию
// @Summary Login user
// @Description Authenticate user and set a session cookie
// @Tags auth
// @Accept json
// @Produce json
// @Param login body loginRequest true "Login request"
// @Success 200 {object} store.User
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /login [post]
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var request loginRequest
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		apiutils.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if request.Email == "" {
		apiutils.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "no email provided"})
		return
	}
	if request.Password == "" {
		apiutils.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "no password provided"})
		return
	}

	user, err := h.Store.AuthenticateUser(request.Email, request.Password)
	if err != nil {
		apiutils.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": fmt.Sprintf("error authenticating user: %s", err)})
		return
	}

	sessionID := h.Store.CreateSession(user.ID)
	expiration := time.Now().Add(sessionDuration)
	session := &http.Cookie{
		Name:     "session_id",
		Value:    sessionID,
		Expires:  expiration,
		HttpOnly: true,
	}
	http.SetCookie(w, session)

	apiutils.WriteJSON(w, http.StatusOK, user)
}

// Logout удаляет сессию (cookie) пользователя
// @Summary Logout user
// @Description Delete user's session and invalidate cookie
// @Tags auth
// @Produce json
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /logout [post]
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	session, err := r.Cookie("session_id")
	if errors.Is(err, http.ErrNoCookie) {
		log.Info().Msg("no session cookie found")
		apiutils.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "no session cookie"})
		return
	}
	if err != nil {
		log.Error().Err(err).Msg("error getting session cookie")
		apiutils.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	h.Store.DeleteSession(session.Value)
	session.Expires = time.Now().Add(-1 * time.Hour)
	http.SetCookie(w, session)

	apiutils.WriteJSON(w, http.StatusOK, errorResponse{"logged out"})
}

type sessionResponse struct {
	Authenticated bool        `json:"authenticated"`
	User          *store.User `json:"user,omitempty"`
}

// CheckSession проверяет валидность сессии и возвращает пользователя
// @Summary Check current session
// @Description Return user for current session cookie or null if not authenticated
// @Tags auth
// @Produce json
// @Success 200 {object} store.User
// @Failure 500 {object} map[string]string
// @Router /session [get]
func (h *Handler) CheckSession(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session_id")
	if errors.Is(err, http.ErrNoCookie) {
		apiutils.WriteJSON(w, http.StatusOK, sessionResponse{Authenticated: false})
		return
	}
	if err != nil {
		log.Error().Err(err).Msg("error reading session cookie")
		apiutils.WriteJSON(w, http.StatusInternalServerError, errorResponse{"internal server error"})
		return
	}

	sessionID := cookie.Value

	user, ok := h.Store.GetUserBySession(sessionID)
	if !ok {
		log.Error().Err(err).Msg("error loading user for session")
		apiutils.WriteJSON(w, http.StatusOK, sessionResponse{Authenticated: false})
		return
	}

	apiutils.WriteJSON(w, http.StatusOK, sessionResponse{
		Authenticated: true,
		User:          user,
	})
}

type TestListResponse struct {
	ID                uint64        `json:"id"`
	Name              string        `json:"name"`
	Description       string        `json:"description"`
	TimeLimit         time.Duration `json:"timeLimit"`
	MaxScore          uint64        `json:"maxScore"`
	NumberOfQuestions uint64        `json:"numberOfQuestions"`
}

func (h *Handler) TestById(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	testID, err := strconv.ParseUint(vars["test_id"], 10, 64)
	if err != nil {
		apiutils.WriteJSON(w, http.StatusBadRequest, errorResponse{"invalid test_id"})
		return
	}

	test, ok := h.Store.TestById(testID)

	result := TestListResponse{
		ID:                testID,
		Name:              test.Name,
		Description:       test.Description,
		TimeLimit:         test.TimeLimit,
		MaxScore:          test.MaxScore,
		NumberOfQuestions: uint64(len(test.Questions)),
	}

	if !ok {
		apiutils.WriteJSON(w, http.StatusBadRequest, errorResponse{"test does not exist"})
	}

	apiutils.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) StartAttempt(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	testID, err := strconv.ParseUint(vars["test_id"], 10, 64)
	if err != nil {
		apiutils.WriteJSON(w, http.StatusBadRequest, errorResponse{"invalid test_id"})
		return
	}

	userId, ok := mw.GetUserID(r.Context())

	if !ok {
		apiutils.WriteJSON(w, http.StatusBadRequest, errorResponse{"invalid user_id"})
	}

	userAttempt, err := h.Store.CreateAttempt(userId, testID)
	if err != nil {
		apiutils.WriteJSON(w, http.StatusInternalServerError, errorResponse{"internal server error"})
	}
	apiutils.WriteJSON(w, http.StatusOK, userAttempt)
}

func (h *Handler) GetAttemptQuestions(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	attemptID, err := strconv.ParseUint(vars["attempt_id"], 10, 64)
	if err != nil {
		apiutils.WriteJSON(w, http.StatusBadRequest, errorResponse{"invalid attempt_id"})
		return
	}

	questions, err := h.Store.GetAttemptQuestions(attemptID)

	if err != nil {
		apiutils.WriteJSON(w, http.StatusInternalServerError, errorResponse{err.Error()})
	}

	apiutils.WriteJSON(w, http.StatusOK, questions)
}

type PostAnswerRequest struct {
	Text string `json:"text"`
}

func (h *Handler) PostQuestionAnswer(w http.ResponseWriter, r *http.Request) {
	var request PostAnswerRequest
	err := json.NewDecoder(r.Body).Decode(&request)

	if err != nil {
		apiutils.WriteJSON(w, http.StatusBadRequest, errorResponse{"invalid request"})
	}

	vars := mux.Vars(r)
	attemptID, err := strconv.ParseUint(vars["attempt_id"], 10, 64)

	if err != nil {
		apiutils.WriteJSON(w, http.StatusBadRequest, errorResponse{"invalid attempt_id"})
	}

	questionID, err := strconv.ParseUint(vars["question_id"], 10, 64)

	if err != nil {
		apiutils.WriteJSON(w, http.StatusBadRequest, errorResponse{"invalid question_id"})
	}

	answer, err := h.Store.CreateAnswer(attemptID, questionID, request.Text)

	if err != nil {
		apiutils.WriteJSON(w, http.StatusInternalServerError, errorResponse{err.Error()})
	}

	apiutils.WriteJSON(w, http.StatusOK, answer)
}

func (h *Handler) SubmitAttempt(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	attemptID, err := strconv.ParseUint(vars["attempt_id"], 10, 64)

	if err != nil {
		apiutils.WriteJSON(w, http.StatusBadRequest, errorResponse{"invalid attempt_id"})
	}

	attempt, err := h.Store.SubmitAttempt(attemptID)

	if err != nil {
		apiutils.WriteJSON(w, http.StatusInternalServerError, errorResponse{err.Error()})
	}

	apiutils.WriteJSON(w, http.StatusOK, attempt)
}

func (h *Handler) SentMassage(w http.ResponseWriter, r *http.Request) {

}

func (h *Handler) NewDialoge(w http.ResponseWriter, r *http.Request) {

}
