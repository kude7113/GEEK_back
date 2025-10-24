package middleware

import (
	"GEEK_back/apiutils"
	"GEEK_back/store"
	"context"
	"errors"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
	"net/http"
	"os"
	"strings"
)

type ctxKey string

const UserIDKey ctxKey = "userID"

func WithUserID(ctx context.Context, id uint64) context.Context {
	return context.WithValue(ctx, UserIDKey, id)
}

func GetUserID(ctx context.Context) (uint64, bool) {
	value := ctx.Value(UserIDKey)
	if value == nil {
		return 0, false
	}
	id, ok := value.(uint64)
	return id, ok
}

// getAllowedOrigins возвращает карту разрешенных origins
func getAllowedOrigins() map[string]bool {
	// Базовые origins для разработки
	allowed := map[string]bool{
		"http://localhost:8080":     true,
		"http://127.0.0.1:8080":     true,
		"http://localhost:8030":     true,
		"http://127.0.0.1:8030":     true,
		"http://0.0.0.0:8030":       true,
		"http://192.168.1.126:3000": true,
		"http://localhost:3000":     true,
	}

	// Добавляем origins из переменной окружения ALLOWED_ORIGINS
	// Формат: http://example.com:8030,http://192.168.1.100:8030
	if envOrigins := os.Getenv("ALLOWED_ORIGINS"); envOrigins != "" {
		for _, origin := range strings.Split(envOrigins, ",") {
			origin = strings.TrimSpace(origin)
			if origin != "" {
				allowed[origin] = true
			}
		}
	}

	return allowed
}

func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowed := getAllowedOrigins()

		if allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func AuthMiddleware(s *store.Store) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			session, err := r.Cookie("session_id")
			if errors.Is(err, http.ErrNoCookie) {
				log.Info().Msg("no session cookie found in auth middleware")
				apiutils.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "no session cookie"})
				return
			}
			if err != nil {
				log.Error().Err(err).Msg("error getting session cookie in auth middleware")
				apiutils.WriteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
				return
			}

			user, ok := s.GetUserBySession(session.Value)
			if !ok {
				apiutils.WriteJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid session"})
				return
			}

			ctx := WithUserID(r.Context(), user.ID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
