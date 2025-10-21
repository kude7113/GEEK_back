package router

import (
	"GEEK_back/client/openAI"
	"GEEK_back/handler"
	mw "GEEK_back/middleware"
	"GEEK_back/store"
	"github.com/gorilla/mux"
	httpSwagger "github.com/swaggo/http-swagger"
	"net/http"
)

func NewRouter(s *store.Store, o *openai.Client) http.Handler {
	h := handler.NewHandler(s, o)

	r := mux.NewRouter()

	r.PathPrefix("/swagger/").Handler(httpSwagger.WrapHandler)

	api := r.PathPrefix("/api").Subrouter()
	protected := api.PathPrefix("").Subrouter()
	protected.Use(mw.AuthMiddleware(s))

	// user routes
	api.HandleFunc("/register", h.Register).Methods("POST")
	api.HandleFunc("/login", h.Login).Methods("POST")
	api.HandleFunc("/logout", h.Logout).Methods("POST")
	api.HandleFunc("/session", h.CheckSession).Methods("GET")

	// tests routes
	//protected.HandleFunc("/test", h.ListTests).Methods("GET")  // закомментировано

	protected.HandleFunc("/test/{test_id}", h.TestById).Methods("GET")
	protected.HandleFunc("/tests/{test_id}/attempt", h.StartAttempt).Methods("POST")
	protected.HandleFunc("/tests/{test_id}/attempts/history", h.GetAttemptHistory).Methods("GET")

	// attempts routes
	protected.HandleFunc("/attempt/{attempt_id}/question", h.GetAttemptQuestions).Methods("GET")
	protected.HandleFunc("/attempt/{attempt_id}/question/{question_position}", h.GetAttemptQuestions).Methods("GET")
	//protected.HandleFunc("/attempts/{attempt_id}/answers", h.ListAnswers).Methods("GET") // закомментировано
	//protected.HandleFunc("/attempts/{attempt_id}/answers/{question_id}", h.GetQuestionAnswer).Methods("GET") // закомментировано
	protected.HandleFunc("/attempt/{attempt_id}/question/{question_position}/submit", h.PostQuestionAnswer).Methods("POST")
	protected.HandleFunc("/attempt/{attempt_id}/submit", h.SubmitAttempt).Methods("POST")

	ai := protected.PathPrefix("/attempt/{attempt_id}/question/{question_position}/ai").Subrouter()

	ai.HandleFunc("/start", h.NewDialoge).Methods("POST")
	ai.HandleFunc("/{thread_id}/send", h.SentMassage).Methods("POST")

	return mw.CORS(r)
}
