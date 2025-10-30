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

type AccessCode struct {
	Code      string     `json:"code"`       // сам код доступа
	TestID    uint64     `json:"test_id"`    // к какому тесту относится
	MaxUses   *uint64    `json:"max_uses"`   // nil = бесконечный, число = ограничение
	UsedCount uint64     `json:"used_count"` // сколько раз использован
	ExpiresAt *time.Time `json:"expires_at"` // nil = не истекает
	CreatedAt time.Time  `json:"created_at"`
}

type Store struct {
	mu           sync.RWMutex
	users        map[uint64]*User
	usersByEmail map[string]uint64
	tests        map[uint64]*Test
	attempts     map[uint64]*Attempt
	sessions     map[string]uint64
	aiThreads    map[uint64]*AIThread
	accessCodes  map[string]*AccessCode // key = код доступа
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
	RightOrNot bool      `json:"right_or_no"`
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
	ID         uint64 `json:"id"`
	Name       string `json:"name"`
	Text       string `json:"text"`
	TrueAnswer string `json:"answer"`
	MaxScore   uint64 `json:"maxScore"`
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
				ID: 1,
				Text: `Посчитать точное количество гласных букв в гимне Российской федерации
						за вычетом буквы 'о', ответ вывести по такой формуле
						X (количество гласных букв) - Y (количество   букв 'о') = Z`,
				TrueAnswer: "270",
				MaxScore:   10,
			},
			{
				ID: 2,
				Text: `Определи что за источник, напиши точную дату публикации и время выхода новости:
						'С января по сентябрь самая высокая доходность в рублях была у корпоративных облигаций.
						 Но отдельно по итогам сентября на первое место по доходности вышел другой актив'`,
				TrueAnswer: "РБК",
				MaxScore:   10,
			},
			{
				ID: 3,
				Text: `Рассчитать  beta = Cov (Ra, Rp)/Var(Ra) для невозобновляемых ресурсов в монголии
						 по 5 разным показателям на основе данных на 2025 год world bank group`,
				TrueAnswer: "2334",
				MaxScore:   10,
			},
			{
				ID:         4,
				Text:       "расставь знаки припинания: научно-технический прогресс не социальный принесёт счастья если не будет дополняться чрезвычайно глубокими изменениями в социальной нравственной и культурной жизни человечества внутреннюю духовную жизнь людей внутренние импульсы их активности трудней всего прогнозировать но именно от этого зависит в конечном итоге и гибель и спасение цивилизации",
				TrueAnswer: "Научно-технический прогресс не социальный принесёт счастья, если не будет дополняться чрезвычайно глубокими изменениями в социальной, нравственной и культурной жизни человечества. Внутреннюю духовную жизнь людей, внутренние импульсы их активности трудней всего прогнозировать, но именно от этого зависит в конечном итоге и гибель, и спасение цивилизации.",
				MaxScore:   10,
			},
			{
				ID:         5,
				Text:       "В комнате находятся Анна, Борис, Василий и Галина. Известно,\n1. Если Анна не брала конфету, то её взял Борис\n2. Если Василий не брал конфету, то Галина тоже её не брала\n3. Ровно один человек взял конфету",
				TrueAnswer: "анна взяла конфету",
				MaxScore:   10,
			},
			{
				ID:         6,
				Text:       "Двойная звезда имеет период Т = 3 года, а расстояние L между ее компонентами равно двум астрономическим единицам. Вырази массу звезды через массу Солнца и сократи до 2 знака после запятой",
				TrueAnswer: "0,89",
				MaxScore:   10,
			},
			{
				ID:         7,
				Text:       "Какая была ключевая ставка ЦБ РФ 22.08.1995",
				TrueAnswer: "180",
				MaxScore:   10,
			},
		},
		NumOfQuestions: 4,
	}

	s.tests[test.ID] = &test

	// Создаем тестовый бесконечный код доступа для test 1
	_, err = s.CreateAccessCode("TEST-2025-INFINITY", test.ID, nil, nil)
	if err != nil {
		return fmt.Errorf("init fill store: failed to create access code: %w", err)
	}

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
		accessCodes:  make(map[string]*AccessCode),
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

	test, ok := s.tests[attempt.TestID]
	if !ok {
		return nil, errors.New("test not found")
	}

	if len(attempt.Answers) < int(questionPos-1) {
		return nil, errors.New("question position out of range")
	}

	question := test.Questions[questionPos-1]
	trueAnswer := question.TrueAnswer

	if text == trueAnswer {
		attempt.Result += question.MaxScore
		attempt.Answers[questionPos-1].RightOrNot = true
	} else {
		attempt.Answers[questionPos-1].RightOrNot = false
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

func (s *Store) GetAttemptByID(attemptID uint64) (*Attempt, bool) {
	attempt, ok := s.attempts[attemptID]
	if !ok {
		return nil, false
	}

	return attempt, true
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
	if questionPosition != 1 {
		if _, exists := s.aiThreads[key]; exists {
			return nil, errors.New("thread already exists for this question")
		}
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

// CreateAccessCode создает новый код доступа для теста
func (s *Store) CreateAccessCode(code string, testID uint64, maxUses *uint64, expiresAt *time.Time) (*AccessCode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Проверяем, что тест существует
	if _, ok := s.tests[testID]; !ok {
		return nil, errors.New("test not found")
	}

	// Проверяем, что код не существует
	if _, ok := s.accessCodes[code]; ok {
		return nil, errors.New("access code already exists")
	}

	accessCode := &AccessCode{
		Code:      code,
		TestID:    testID,
		MaxUses:   maxUses,
		UsedCount: 0,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now().UTC(),
	}

	s.accessCodes[code] = accessCode

	return accessCode, nil
}

// ValidateAccessCode проверяет код доступа и увеличивает счетчик использования
func (s *Store) ValidateAccessCode(code string, testID uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	accessCode, ok := s.accessCodes[code]
	if !ok {
		return errors.New("invalid access code")
	}

	// Проверяем, что код для нужного теста
	if accessCode.TestID != testID {
		return errors.New("access code is not valid for this test")
	}

	// Проверяем срок действия
	if accessCode.ExpiresAt != nil && time.Now().UTC().After(*accessCode.ExpiresAt) {
		return errors.New("access code has expired")
	}

	// Проверяем лимит использований
	if accessCode.MaxUses != nil && accessCode.UsedCount >= *accessCode.MaxUses {
		return errors.New("access code usage limit reached")
	}

	// Увеличиваем счетчик использования
	accessCode.UsedCount++

	return nil
}

// GetUserAttemptHistory возвращает историю завершенных попыток пользователя для указанного теста
func (s *Store) GetUserAttemptHistory(userID, testID uint64) ([]*Attempt, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Проверяем, что тест существует
	if _, ok := s.tests[testID]; !ok {
		return nil, errors.New("test not found")
	}

	var history []*Attempt

	// Проходим по всем попыткам и фильтруем по userID, testID и статусу
	for _, attempt := range s.attempts {
		if attempt.UserID == userID && attempt.TestID == testID && attempt.Status == "submitted" {
			history = append(history, attempt)
		}
	}

	// Сортируем от новых к старым (по времени завершения)
	for i := 0; i < len(history); i++ {
		for j := i + 1; j < len(history); j++ {
			if history[i].FinishedAt.Before(history[j].FinishedAt) {
				history[i], history[j] = history[j], history[i]
			}
		}
	}

	return history, nil
}
