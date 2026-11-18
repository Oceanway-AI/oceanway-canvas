package app

import "yingce/backend/internal/model"

func (s *Service) RequireAdmin(user *model.User) error {
	if user == nil {
		return Unauthorized("请先登录")
	}
	if !user.CanStudio(model.StudioManageCreativeContent) {
		return Forbidden("当前账号没有 Studio 内容管理权限")
	}
	return nil
}
