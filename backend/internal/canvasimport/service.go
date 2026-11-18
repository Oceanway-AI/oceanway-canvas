package canvasimport

import (
	"yingce/backend/internal/model"
	"yingce/backend/internal/repository"
)

// Host supplies the Studio content permission and encrypted import settings.
type Host interface {
	RequireAdmin(*model.User) error
	EncryptSecret(string) (string, error)
	DecryptSecret(string) (string, error)
	BrandName() string
}

type Service struct {
	repo *repository.Repository
	host Host
}

func New(repo *repository.Repository, host Host) *Service {
	return &Service{repo: repo, host: host}
}
