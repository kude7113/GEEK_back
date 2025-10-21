package handler

import (
	"GEEK_back/apiutils"
	openai "GEEK_back/client/openAI"
	mw "GEEK_back/middleware"
	"GEEK_back/store"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"

	"github.com/rs/zerolog/log"
)

const sessionDuration = 24 * time.Hour

type Handler struct {
	Store  *store.Store
	Openai *openai.Client
}

type errorResponse struct {
	Error string `json:"error"`
}

func NewHandler(s *store.Store, o *openai.Client) *Handler {
	return &Handler{
		Store:  s,
		Openai: o,
	}
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
		Secure:   false, // false для работы по HTTP
		SameSite: http.SameSiteLaxMode, // Lax для cross-origin по HTTP
		Path:     "/",
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

// TestById возвращает тест по ID
// @Summary Get test by ID
// @Description Retrieves a test by its ID
// @Param test_id path int true "Test ID"
// @Success 200 {object} store.Test
// @Failure 400 {object} map[string]string
// @Router /test/{test_id} [get]
func (h *Handler) TestById(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	testID, err := strconv.ParseUint(vars["test_id"], 10, 64)
	if err != nil {
		apiutils.WriteJSON(w, http.StatusBadRequest, errorResponse{"invalid test_id"})
		return
	}

	test, ok := h.Store.TestById(testID)
	if !ok {
		apiutils.WriteJSON(w, http.StatusBadRequest, errorResponse{"test does not exist"})
	}

	testWithoutQuestions := *test
	testWithoutQuestions.Questions = nil

	apiutils.WriteJSON(w, http.StatusOK, testWithoutQuestions)
}

type startAttemptRequest struct {
	AccessCode string `json:"access_code"`
}

// StartAttempt начинает попытку теста
// @Summary Start test attempt
// @Description Starts a new attempt for the given test with access code validation
// @Param test_id path int true "Test ID"
// @Param access_code body startAttemptRequest true "Access code for the test"
// @Success 200 {object} store.Attempt
// @Failure 400 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /tests/{test_id}/attempt [post]
func (h *Handler) StartAttempt(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	testID, err := strconv.ParseUint(vars["test_id"], 10, 64)
	if err != nil {
		apiutils.WriteJSON(w, http.StatusBadRequest, errorResponse{"invalid test_id"})
		return
	}

	// Читаем access code из body
	var request startAttemptRequest
	err = json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		apiutils.WriteJSON(w, http.StatusBadRequest, errorResponse{"invalid json"})
		return
	}

	if request.AccessCode == "" {
		apiutils.WriteJSON(w, http.StatusBadRequest, errorResponse{"access code is required"})
		return
	}

	// Валидируем код доступа
	err = h.Store.ValidateAccessCode(request.AccessCode, testID)
	if err != nil {
		apiutils.WriteJSON(w, http.StatusForbidden, errorResponse{err.Error()})
		return
	}

	userId, ok := mw.GetUserID(r.Context())
	if !ok {
		apiutils.WriteJSON(w, http.StatusBadRequest, errorResponse{"invalid user_id"})
		return
	}

	userAttempt, err := h.Store.CreateAttempt(userId, testID)
	if err != nil {
		apiutils.WriteJSON(w, http.StatusInternalServerError, errorResponse{"internal server error"})
		return
	}
	apiutils.WriteJSON(w, http.StatusOK, userAttempt)
}

// GetAttemptQuestions получает вопросы для попытки
// @Summary Get questions for test attempt
// @Description Retrieves all questions for the specified attempt
// @Param attempt_id path int true "Attempt ID"
// @Success 200 {array} store.Question
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /attempt/{attempt_id}/question [get]
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

// PostQuestionAnswer отправляет ответ на вопрос
// @Summary Submit an answer for a question
// @Description Submits the answer for a given question in the attempt
// @Param attempt_id path int true "Attempt ID"
// @Param question_position path int true "Question Position"
// @Param text body PostAnswerRequest true "Answer text"
// @Success 200 {object} store.Answer
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /attempt/{attempt_id}/question/{question_position}/submit [post]
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

	questionPos, err := strconv.ParseUint(vars["question_position"], 10, 64)

	if err != nil {
		apiutils.WriteJSON(w, http.StatusBadRequest, errorResponse{"invalid question_id"})
	}

	answer, err := h.Store.CreateAnswer(attemptID, questionPos, request.Text)

	if err != nil {
		apiutils.WriteJSON(w, http.StatusInternalServerError, errorResponse{err.Error()})
	}

	apiutils.WriteJSON(w, http.StatusOK, answer)
}

// SubmitAttempt завершает попытку
// @Summary Submit the attempt and evaluate the result
// @Description Submits the entire attempt and evaluates the score
// @Param attempt_id path int true "Attempt ID"
// @Success 200 {object} store.Attempt
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /attempt/{attempt_id}/submit [post]
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
	vars := mux.Vars(r)

	threadID := vars["thread_id"]
	if threadID == "" {
		apiutils.WriteJSON(w, http.StatusBadRequest, errorResponse{"thread_id is required"})
		return
	}

	attemptID, err := strconv.ParseUint(vars["attempt_id"], 10, 64)
	if err != nil {
		apiutils.WriteJSON(w, http.StatusBadRequest, errorResponse{"invalid attempt_id"})
		return
	}

	// Читаем тело запроса
	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiutils.WriteJSON(w, http.StatusBadRequest, errorResponse{"invalid request body"})
		return
	}

	if req.Message == "" {
		apiutils.WriteJSON(w, http.StatusBadRequest, errorResponse{"message cannot be empty"})
		return
	}

	// Проверяем дедлайн попытки
	if err := h.Store.CheckDeadline(attemptID); err != nil {
		apiutils.WriteJSON(w, http.StatusBadRequest, errorResponse{err.Error()})
		return
	}

	// Добавляем сообщение в тред
	if err := h.Openai.AddMessage(r.Context(), threadID, req.Message); err != nil {
		apiutils.WriteJSON(w, http.StatusInternalServerError, errorResponse{err.Error()})
		return
	}

	// Запускаем ассистента
	run, err := h.Openai.RunAssistant(r.Context(), threadID)
	if err != nil {
		apiutils.WriteJSON(w, http.StatusInternalServerError, errorResponse{err.Error()})
		return
	}

	// Ждем завершения (максимум 30 секунд)
	if err := h.Openai.WaitForCompletion(r.Context(), threadID, run.ID, 30*time.Second); err != nil {
		apiutils.WriteJSON(w, http.StatusInternalServerError, errorResponse{err.Error()})
		return
	}

	// Получаем последние сообщения
	messages, err := h.Openai.GetMessages(r.Context(), threadID, 1)
	if err != nil {
		apiutils.WriteJSON(w, http.StatusInternalServerError, errorResponse{"failed to get response"})
		return
	}

	if len(messages) == 0 {
		apiutils.WriteJSON(w, http.StatusInternalServerError, errorResponse{"no response from assistant"})
		return
	}

	// Извлекаем текст ответа
	assistantMessage := messages[0]
	var responseText string
	if len(assistantMessage.Content) > 0 && assistantMessage.Content[0].Text != nil {
		responseText = assistantMessage.Content[0].Text.Value
	}

	// Возвращаем ответ
	apiutils.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"response": responseText,
	})
}

func (h *Handler) NewDialoge(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	attemptID, err := strconv.ParseUint(vars["attempt_id"], 10, 64)
	if err != nil {
		apiutils.WriteJSON(w, http.StatusBadRequest, errorResponse{"invalid attempt_id"})
		return
	}

	questionPos, err := strconv.ParseUint(vars["question_position"], 10, 64)
	if err != nil {
		apiutils.WriteJSON(w, http.StatusBadRequest, errorResponse{"invalid question_position"})
		return
	}

	// Создаем thread в OpenAI
	threadID, err := h.Openai.CreateThread(r.Context())
	if err != nil {
		apiutils.WriteJSON(w, http.StatusInternalServerError, errorResponse{err.Error()})
		return
	}

	// Сохраняем в Store
	thread, err := h.Store.CreateAIThread(attemptID, questionPos, threadID)
	if err != nil {
		apiutils.WriteJSON(w, http.StatusInternalServerError, errorResponse{err.Error()})
		return
	}

	// Возвращаем успешный ответ
	apiutils.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"thread_id":  thread.ThreadID,
		"attempt_id": thread.AttemptID,
		"status":     thread.Status,
	})
}

type Results struct {
	Score   uint64          `json:"score"`
	Answers []*store.Answer `json:"answers"`
}

func (h *Handler) GetAttemptResults(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	attemptID, err := strconv.ParseUint(vars["attempt_id"], 10, 64)
	if err != nil {
		apiutils.WriteJSON(w, http.StatusBadRequest, errorResponse{"invalid attempt_id"})
	}

	attempt, ok := h.Store.GetAttemptByID(attemptID)
	if !ok {
		apiutils.WriteJSON(w, http.StatusBadRequest, errorResponse{"invalid attempt_id"})
	}

	apiutils.WriteJSON(w, http.StatusOK, Results{
		Score:   attempt.Result,
		Answers: attempt.Answers,
	})
}

// GetAttemptHistory возвращает историю завершенных попыток пользователя для теста
// @Summary Get user's attempt history for a test
// @Description Retrieves all completed attempts for the current user and specified test
// @Tags attempts
// @Produce json
// @Param test_id path int true "Test ID"
// @Success 200 {array} store.Attempt
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /tests/{test_id}/attempts/history [get]
// @Security CookieAuth
func (h *Handler) GetAttemptHistory(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	testID, err := strconv.ParseUint(vars["test_id"], 10, 64)
	if err != nil {
		apiutils.WriteJSON(w, http.StatusBadRequest, errorResponse{"invalid test_id"})
		return
	}

	userID, ok := mw.GetUserID(r.Context())
	if !ok {
		apiutils.WriteJSON(w, http.StatusBadRequest, errorResponse{"invalid user_id"})
		return
	}

	history, err := h.Store.GetUserAttemptHistory(userID, testID)
	if err != nil {
		apiutils.WriteJSON(w, http.StatusInternalServerError, errorResponse{err.Error()})
		return
	}

	apiutils.WriteJSON(w, http.StatusOK, history)
}
