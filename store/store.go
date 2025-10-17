package store

import (
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserExists             = errors.New("user already exists")
	ErrInvalidEmailOrPassword = errors.New("invalid email or password")
)

type Store struct {
	mu           sync.RWMutex
	users        map[uint64]*User
	usersByEmail map[string]uint64
	tests        map[uint64]*Test
	attempts     map[uint64]*Attempt
	sessions     map[string]uint64
	aiThreads    map[uint64]*AIThread
	nextUserID   uint64
}

type User struct {
	ID        uint64    `json:"id"`
	Email     string    `json:"email"`
	Password  string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

type AIThread struct {
	AttemptID uint64    `json:"attempt_id"`
	ThreadID  string    `json:"thread_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type Answer struct {
	ID         uint64    `json:"id"`
	QuestionID uint64    `json:"question_id"`
	Text       string    `json:"text"`
	CreatedAt  time.Time `json:"created_at"`
}

type Attempt struct {
	ID         uint64    `json:"id"`
	UserID     uint64    `json:"user_id"`
	TestID     uint64    `json:"test_id"`
	Status     string    `json:"status"`
	Answers    []*Answer `json:"answers"`
	Result     uint64    `json:"result"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
}

type Question struct {
	ID       uint64 `json:"id"`
	Name     string `json:"name"`
	Text     string `json:"text"`
	MaxScore uint64 `json:"maxScore"`
}

type Test struct {
	ID             uint64        `json:"id"`
	Name           string        `json:"name"`
	Description    string        `json:"description"`
	TimeLimit      time.Duration `json:"timeLimit"`
	MaxScore       uint64        `json:"maxScore"`
	Questions      []*Question   `json:"questions,omitempty"`
	NumOfQuestions uint64        `json:"numOfQuestions"` // Количество вопросов, которые нужно выбрать для попытки
}

func (s *Store) InitFillStore() error {
	_, err := s.CreateUser("user@test.test", "test")
	if err != nil {
		return fmt.Errorf("init fill store: %w", err)
	}

	test := Test{
		ID:          1,
		Name:        "test 1",
		Description: "description for test 1",
		TimeLimit:   time.Hour * 1,
		MaxScore:    100,
		Questions: []*Question{
			{
				ID:       1,
				Name:     "question 1",
				Text:     "text question 1",
				MaxScore: 10,
			},
			{
				ID:       2,
				Name:     "question 2",
				Text:     "text question 2",
				MaxScore: 10,
			},
			{
				ID:       3,
				Name:     "question 3",
				Text:     "text question 3",
				MaxScore: 10,
			},
			{
				ID:       4,
				Name:     "question 4",
				Text:     "text question 4",
				MaxScore: 10,
			},
		},
		NumOfQuestions: 2,
	}

	s.tests[test.ID] = &test

	return nil
}

func NewStore() *Store {
	return &Store{
		users:        make(map[uint64]*User),
		tests:        make(map[uint64]*Test),
		attempts:     make(map[uint64]*Attempt),
		usersByEmail: make(map[string]uint64),
		sessions:     make(map[string]uint64),
		aiThreads:    make(map[uint64]*AIThread),
		nextUserID:   1,
	}
}

func (s *Store) CreateUser(email, password string) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.usersByEmail[email]; ok {
		return nil, ErrUserExists
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("cannot hash password: %w", err)
	}

	user := &User{
		ID:        s.nextUserID,
		Email:     email,
		Password:  string(hashedPassword),
		CreatedAt: time.Now().UTC(),
	}
	s.users[user.ID] = user
	s.usersByEmail[email] = user.ID
	s.nextUserID++

	return user, nil
}

func (s *Store) CreateAttempt(testID, userID uint64) (*Attempt, error) {
	test, exists := s.tests[testID]
	if !exists {
		return nil, fmt.Errorf("test not found")
	}

	// Выбираем случайные вопросы
	selectedQuestions := s.getRandomQuestions(test.Questions, test.NumOfQuestions)

	// Создаем новую попытку
	attempt := &Attempt{
		ID:        uint64(len(s.attempts)) + 1,
		UserID:    userID,
		TestID:    testID,
		Status:    "started", // Статус попытки
		Answers:   make([]*Answer, len(selectedQuestions)),
		StartedAt: time.Now().UTC(),
	}

	// Здесь можно добавить логику для создания ответов для выбранных вопросов
	for i, question := range selectedQuestions {
		// Это можно заменить на логику создания ответа на вопрос
		attempt.Answers[i] = &Answer{
			ID:         question.ID,
			QuestionID: question.ID,
			Text:       "", // Ответ будет пустым до завершения попытки
		}
	}

	s.mu.Lock()
	s.attempts[attempt.ID] = attempt
	s.nextUserID++
	s.mu.Unlock()

	return attempt, nil
}

// Функция для получения случайных вопросов
func (s *Store) getRandomQuestions(allQuestions []*Question, numOfQuestions uint64) []*Question {
	source := rand.NewSource(time.Now().UnixNano())
	r := rand.New(source)

	r.Shuffle(len(allQuestions), func(i, j int) {
		allQuestions[i], allQuestions[j] = allQuestions[j], allQuestions[i]
	})

	if numOfQuestions > uint64(len(allQuestions)) {
		numOfQuestions = uint64(len(allQuestions))
	}

	return allQuestions[:numOfQuestions]
}

func (s *Store) AuthenticateUser(email, password string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	userID, ok := s.usersByEmail[email]
	if !ok {
		return nil, ErrInvalidEmailOrPassword
	}
	user := s.users[userID]

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return nil, ErrInvalidEmailOrPassword
	}

	return user, nil
}

func (s *Store) CreateSession(userID uint64) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessionID := uuid.NewString()
	s.sessions[sessionID] = userID

	return sessionID
}

func (s *Store) DeleteSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, sessionID)
}

func (s *Store) GetUserBySession(sessionID string) (*User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	userID, ok := s.sessions[sessionID]
	if !ok {
		log.Info().Str("session_id", sessionID).Msg("session not found")
		return nil, false
	}
	user, ok := s.users[userID]

	return user, ok
}

func (s *Store) TestById(testId uint64) (*Test, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result, ok := s.tests[testId]
	if !ok {
		log.Info().Str("testId", fmt.Sprintf("%d", testId)).Msg("test not found")
	}

	return result, ok
}

func (s *Store) GetAttemptQuestions(attemptId uint64) ([]*Question, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	attempt, ok := s.attempts[attemptId]
	if !ok {
		return nil, errors.New("attempt not found")
	}

	// Собираем вопросы из попытки
	var questions []*Question
	for _, answer := range attempt.Answers {
		// Ищем вопрос по ID
		question, ok := s.findQuestionByID(attempt.TestID, answer.QuestionID)
		if !ok {
			return nil, errors.New("question not found for answer")
		}
		questions = append(questions, question)
	}

	return questions, nil
}

func (s *Store) findQuestionByID(testID, questionID uint64) (*Question, bool) {
	test, ok := s.tests[testID]
	if !ok {
		return nil, false
	}

	for _, question := range test.Questions {
		if question.ID == questionID {
			return question, true
		}
	}

	return nil, false
}

func (s *Store) CheckDeadline(attemptID uint64) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	attempt, ok := s.attempts[attemptID]
	if !ok {
		return errors.New("attempt not found")
	}

	test, ok := s.tests[attempt.TestID]
	if !ok {
		return errors.New("test not found")
	}

	if test.TimeLimit > 0 {
		deadline := attempt.StartedAt.Add(test.TimeLimit)
		if time.Now().UTC().After(deadline) {
			return errors.New("test attempt timeout")
		}
	}

	return nil
}

func (s *Store) CreateAnswer(attemptID uint64, questionPos uint64, text string) (*Answer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	err := s.CheckDeadline(attemptID)
	if err != nil {
		return nil, err
	}

	attempt, ok := s.attempts[attemptID]
	if !ok {
		return nil, errors.New("attempt not found")
	}

	if len(attempt.Answers) < int(questionPos-1) {
		return nil, errors.New("question position out of range")
	}

	attempt.Answers[questionPos-1].Text = text
	attempt.Answers[questionPos-1].CreatedAt = time.Now().UTC()

	return attempt.Answers[questionPos-1], nil
}

func (s *Store) SubmitAttempt(attemptID uint64) (*Attempt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	err := s.CheckDeadline(attemptID)
	if err != nil {
		return nil, err
	}

	if s.attempts[attemptID].Status != "started" {
		return nil, errors.New("attempt closed")
	}

	s.attempts[attemptID].Status = "submitted"
	s.attempts[attemptID].FinishedAt = time.Now().UTC()
	attempt, ok := s.attempts[attemptID]
	if !ok {
		return nil, errors.New("attempt not found")
	}
	return attempt, nil
}

func (s *Store) CreateAIThread(attemptID, questionPosition uint64, threadID string) (*AIThread, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Проверяем существование attempt
	_, ok := s.attempts[attemptID]
	if !ok {
		return nil, errors.New("attempt not found")
	}

	// Проверяем, что question position валидна
	attempt := s.attempts[attemptID]
	if questionPosition > uint64(len(attempt.Answers)) || questionPosition == 0 {
		return nil, errors.New("invalid question position")
	}

	// Создаем ключ для хранения (attemptID * 1000 + questionPosition)
	// это простой способ создать уникальный ключ из двух чисел
	key := attemptID*1000 + questionPosition

	// Проверяем, что для этого вопроса еще нет диалога
	if _, exists := s.aiThreads[key]; exists {
		return nil, errors.New("thread already exists for this question")
	}

	thread := &AIThread{
		AttemptID: attemptID,
		ThreadID:  threadID,
		Status:    "active",
		CreatedAt: time.Now().UTC(),
	}

	s.aiThreads[key] = thread

	return thread, nil
}
