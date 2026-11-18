package repository

import (
	"yingce/backend/internal/model"
)

func (r *Repository) User(id string) (*model.User, error) {
	var user model.User
	if err := r.db.First(&user, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}
