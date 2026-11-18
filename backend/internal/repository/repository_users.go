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

func (r *Repository) UsersByIDs(ids []string) (map[string]model.User, error) {
	if len(ids) == 0 {
		return map[string]model.User{}, nil
	}
	var users []model.User
	if err := r.db.Select("id", "username", "display_name", "email").Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	result := make(map[string]model.User, len(users))
	for _, user := range users {
		result[user.ID] = user
	}
	return result, nil
}
