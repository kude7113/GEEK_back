package main

import (
	_ "GEEK_back/docs"
	"GEEK_back/router"
	"GEEK_back/store"
	"errors"
	"net/http"

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
	s := store.NewStore()

	if err := s.InitFillStore(); err != nil {
		log.Fatal().Err(err).Msg("failed to init store")
	}

	r := router.NewRouter(s)

	server := &http.Server{
		Addr:    localhost + ":" + port,
		Handler: r,
	}

	log.Info().Str("addr", server.Addr).Msg("listening")
	err := server.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal().Err(err).Msg("server error")
	}
}
