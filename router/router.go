package router

import (
	"GEEK_back/handler"
	mw "GEEK_back/middleware"
	"GEEK_back/store"
	"net/http"

	"github.com/gorilla/mux"
)

func NewRouter(s *store.Store) http.Handler {
	h := handler.NewHandler(s)

	r := mux.NewRouter()

	api := r.PathPrefix("/api").Subrouter()
	protected := api.PathPrefix("").Subrouter()
	protected.Use(mw.AuthMiddleware(s))

	// user
	api.HandleFunc("/register", h.Register).Methods("POST")
	api.HandleFunc("/login", h.Login).Methods("POST")
	api.HandleFunc("/logout", h.Logout).Methods("POST")
	api.HandleFunc("/session", h.CheckSession).Methods("GET")

	// tests
	//protected.HandleFunc("/tests", h.ListTests).Methods("GET")

	protected.HandleFunc("/test/{test_id}", h.TestById).Methods("GET")
	protected.HandleFunc("/tests/{test_id}/attempt", h.StartAttempt).Methods("POST")

	// attempts
	protected.HandleFunc("/attempt/{attempt_id}/question", h.GetAttemptQuestions).Methods("GET")
	protected.HandleFunc("/attempt/{attempt_id}/question/{question_position}", h.GetAttemptQuestions).Methods("GET")
	//protected.HandleFunc("/attempts/{attempt_id}/answers", h.ListAnswers).Methods("GET")
	//protected.HandleFunc("/attempts/{attempt_id}/answers/{question_id}", h.GetQuestionAnswer).Methods("GET")
	protected.HandleFunc("/attempt/{attempt_id}/question/{question_position}/submit", h.PostQuestionAnswer).Methods("POST")
	protected.HandleFunc("/attempt/{attempt_id}/submit", h.SubmitAttempt).Methods("POST")

	ai := protected.PathPrefix("/attempt/{attempt_id}/question/{question_position}/ai").Subrouter()

	ai.HandleFunc("/start", h.NewDialoge).Methods("POST")

	ai.HandleFunc("/{thread_id}/send", h.SentMassage).Methods("POST")

	return mw.CORS(r)
}
