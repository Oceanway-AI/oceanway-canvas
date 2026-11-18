// 管理端系统渠道：增删改、复制、密钥加解密与前台投影。
//
// 渠道密钥只以密文入库；前台投影（publicChannel）永远不包含密钥与上游地址。

package app

import (
	"yingce/backend/internal/model"
)

func (s *Service) PublicSystemChannels() ([]PublicModelChannel, error) {
	channels, err := s.repo.SystemChannels(false)
	if err != nil {
		return nil, err
	}
	result := make([]PublicModelChannel, 0, len(channels))
	for _, channel := range channels {
		items, itemErr := s.repo.ChannelModels(channel.ID, false)
		if itemErr != nil {
			return nil, itemErr
		}
		result = append(result, publicChannel(channel, false, items))
	}
	return result, nil
}

func (s *Service) SystemChannel(id string) (*model.ModelChannel, error) {
	channel, err := s.repo.SystemChannel(id)
	if err != nil {
		return nil, err
	}
	if err := s.decryptSystemChannelSecrets(channel); err != nil {
		return nil, err
	}
	return channel, nil
}

func (s *Service) adminSystemChannel(id string) (*model.ModelChannel, error) {
	channel, err := s.repo.AdminSystemChannel(id)
	if err != nil {
		return nil, err
	}
	if err := s.decryptSystemChannelSecrets(channel); err != nil {
		return nil, err
	}
	return channel, nil
}

func normalizeAdminPage(page int, limit int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return page, limit
}

func (s *Service) encryptSystemChannelSecrets(channel *model.ModelChannel) error {
	apiKey, err := s.encryptSettingSecret(channel.APIKey)
	if err != nil {
		return err
	}
	secretKey, err := s.encryptSettingSecret(channel.SecretKey)
	if err != nil {
		return err
	}
	channel.APIKey = apiKey
	channel.SecretKey = secretKey
	return nil
}

func (s *Service) decryptSystemChannelSecrets(channel *model.ModelChannel) error {
	apiKey, err := s.decryptSettingSecret(channel.APIKey)
	if err != nil {
		return err
	}
	secretKey, err := s.decryptSettingSecret(channel.SecretKey)
	if err != nil {
		return err
	}
	channel.APIKey = apiKey
	channel.SecretKey = secretKey
	return nil
}
