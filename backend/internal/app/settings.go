package app

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const encryptedSettingPrefix = "enc:v1:"

func (s *Service) encryptSettingSecret(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	key, err := s.settingsEncryptionKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(value), nil)
	return encryptedSettingPrefix + base64.RawStdEncoding.EncodeToString(append(nonce, ciphertext...)), nil
}

func (s *Service) decryptSettingSecret(value string) (string, error) {
	if !strings.HasPrefix(value, encryptedSettingPrefix) {
		return value, nil
	}
	payload, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(value, encryptedSettingPrefix))
	if err != nil {
		return "", errors.New("执行密钥密文格式无效")
	}
	key, err := s.settingsEncryptionKey()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(payload) < gcm.NonceSize() {
		return "", errors.New("执行密钥密文长度无效")
	}
	plaintext, err := gcm.Open(nil, payload[:gcm.NonceSize()], payload[gcm.NonceSize():], nil)
	if err != nil {
		return "", errors.New("执行密钥解密失败，请检查存储加密密钥")
	}
	return string(plaintext), nil
}

func (s *Service) settingsEncryptionKey() ([]byte, error) {
	path := filepath.Join(s.dataDir, ".settings-key")
	if data, err := os.ReadFile(path); err == nil && len(data) == 32 {
		return data, nil
	}
	if err := os.MkdirAll(s.dataDir, 0o750); err != nil {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	if err := publishSettingsKey(path, key, os.Link); errors.Is(err, os.ErrExist) {
		return readExistingSettingsKey(path)
	} else if err != nil {
		return nil, err
	}
	return key, nil
}

func readExistingSettingsKey(path string) ([]byte, error) {
	// An older process or the exclusive-create fallback may still be writing.
	for attempt := 0; attempt < 11; attempt++ {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, fmt.Errorf("读取存储加密密钥失败：%w", readErr)
		}
		if len(data) == 32 {
			return data, nil
		}
		if attempt < 10 {
			time.Sleep(5 * time.Millisecond)
		}
	}
	return nil, errors.New("存储加密密钥长度无效")
}

func publishSettingsKey(path string, key []byte, link func(string, string) error) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".settings-key-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := file.Write(key); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	// Link publishes complete bytes without replacing another process's key.
	if err := link(file.Name(), path); err != nil {
		if !errors.Is(err, syscall.ENOTSUP) && !errors.Is(err, syscall.EPERM) {
			return err
		}
		// Some filesystems prohibit hard links. O_EXCL still prevents replacement,
		// but readers may see an incomplete file and a crash may leave it invalid.
		fallback, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		defer fallback.Close()
		if _, err := fallback.Write(key); err != nil {
			return err
		}
		if err := fallback.Sync(); err != nil {
			return err
		}
		if err := fallback.Close(); err != nil {
			return err
		}
	}
	// Persist the published name, not only the inode's contents.
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func (s *Service) protectTaskSecrets(value interface{}) error {
	switch item := value.(type) {
	case map[string]interface{}:
		for key, child := range item {
			if isTaskSecretField(key) {
				secret, _ := child.(string)
				if secret != "" && secret != "system" && !strings.HasPrefix(secret, encryptedSettingPrefix) {
					encrypted, err := s.encryptSettingSecret(secret)
					if err != nil {
						return err
					}
					item[key] = encrypted
				}
				continue
			}
			if err := s.protectTaskSecrets(child); err != nil {
				return err
			}
		}
	case []interface{}:
		for _, child := range item {
			if err := s.protectTaskSecrets(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) decryptTaskInputJSON(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" || !strings.Contains(raw, encryptedSettingPrefix) {
		return raw, nil
	}
	var input interface{}
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		return "", err
	}
	if err := s.decryptTaskSecrets(input); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(input)
	return string(encoded), err
}

func (s *Service) decryptTaskSecrets(value interface{}) error {
	switch item := value.(type) {
	case map[string]interface{}:
		for key, child := range item {
			if isTaskSecretField(key) {
				secret, _ := child.(string)
				if strings.HasPrefix(secret, encryptedSettingPrefix) {
					plain, err := s.decryptSettingSecret(secret)
					if err != nil {
						return err
					}
					item[key] = plain
				}
				continue
			}
			if err := s.decryptTaskSecrets(child); err != nil {
				return err
			}
		}
	case []interface{}:
		for _, child := range item {
			if err := s.decryptTaskSecrets(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func isTaskSecretField(key string) bool {
	switch key {
	case "apiKey", "secretKey", "runningHubWalletApiKey", "runningHubUploadApiKey":
		return true
	default:
		return false
	}
}
