package app

import (
	"errors"

	"gorm.io/gorm"
	"yingce/backend/internal/model"
)

const creativeTestUserIDValue = "creative-test-user"

// creativeTestUserID creates the owner used by isolated creative source tests.
func creativeTestUserID(svc *Service) string {
	if svc == nil || svc.repo == nil {
		return creativeTestUserIDValue
	}
	if _, err := svc.repo.User(creativeTestUserIDValue); err == nil || !errors.Is(err, gorm.ErrRecordNotFound) {
		return creativeTestUserIDValue
	}
	user := &model.User{ID: creativeTestUserIDValue, Username: creativeTestUserIDValue, DisplayName: "创作测试用户", Status: model.UserStatusActive}
	_ = svc.repo.Create(user)
	return creativeTestUserIDValue
}

func ensureTestUsers(t interface{ Fatal(...any) }, db *gorm.DB, ids ...string) {
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if err := db.FirstOrCreate(&model.User{}, model.User{ID: id, Username: id, Status: model.UserStatusActive}).Error; err != nil {
			t.Fatal(err)
		}
	}
}
