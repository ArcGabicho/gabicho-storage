// Package handlers implementa los endpoints REST del servicio.
package handlers

import (
	"github.com/gabicho/gabicho-storage/config"
	"github.com/gabicho/gabicho-storage/db"
	"github.com/gabicho/gabicho-storage/storage"
	"github.com/gabicho/gabicho-storage/utils"
)

// Handler agrupa las dependencias compartidas por todos los endpoints.
type Handler struct {
	Repo    *db.Repository
	Storage *storage.LocalStorage
	Config  *config.Config
	Signer  *utils.TokenSigner
}

// New crea un Handler con sus dependencias.
func New(repo *db.Repository, store *storage.LocalStorage, cfg *config.Config) *Handler {
	return &Handler{
		Repo:    repo,
		Storage: store,
		Config:  cfg,
		Signer:  utils.NewTokenSigner(cfg.SigningSecret),
	}
}
