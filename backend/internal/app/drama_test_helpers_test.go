package app

import (
	"testing"

	"yingce/backend/internal/model"
)

func mustDramaProduction(t *testing.T, svc *Service, project string) *model.DramaProduction {
	t.Helper()
	p, err := svc.repo.DramaProduction(project)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
