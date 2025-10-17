package main

import (
	"GEEK_back/client/openAI"
	_ "GEEK_back/docs"
	"GEEK_back/router"
	"GEEK_back/store"
	"errors"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
)

const localhost = "localhost"
const host = "0.0.0.0"
const port = "8080"

// @title GEEK API
// @version 1.0
// @description API for web-site GEEK
// @BasePath /api
// @securityDefinitions.apikey CookieAuth
// @in header
// @name Cookie
func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Error loading .env file")

	}

	s := store.NewStore()

	if err := s.InitFillStore(); err != nil {
		log.Fatal().Err(err).Msg("failed to init store")
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal().Msg("OPENAI_API_KEY is not set")
	}

	assistantID := os.Getenv("OPENAI_ASSISTANT_ID")
	if assistantID == "" {
		log.Fatal().Msg("OPENAI_ASSISTANT_ID is not set")
	}

	o := openai.NewClient(apiKey, assistantID)

	r := router.NewRouter(s, o)

	server := &http.Server{
		Addr:    host + ":" + port,
		Handler: r,
	}

	log.Info().Str("addr", server.Addr).Msg("listening")
	err = server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal().Err(err).Msg("server error")
	}
}
