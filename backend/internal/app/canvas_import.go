package app

import (
	"crypto/rand"
	"encoding/hex"
	"yingce/backend/internal/canvasimport"
	"yingce/backend/internal/model"
)

type (
	LibTVSettingRequest    = canvasimport.LibTVSettingRequest
	PublicLibTVSetting     = canvasimport.PublicLibTVSetting
	LibTVImportRequest     = canvasimport.LibTVImportRequest
	LibTVImportResult      = canvasimport.LibTVImportResult
	LibTVCanvasConnection  = canvasimport.LibTVCanvasConnection
	LibTVCanvasNode        = canvasimport.LibTVCanvasNode
	LibTVImportIssue       = canvasimport.LibTVImportIssue
	LibTVImportMetadata    = canvasimport.LibTVImportMetadata
	LibTVImportWarning     = canvasimport.LibTVImportWarning
	TapNowImportRequest    = canvasimport.TapNowImportRequest
	TapNowImportResult     = canvasimport.TapNowImportResult
	TapNowCanvasConnection = canvasimport.TapNowCanvasConnection
	TapNowCanvasNode       = canvasimport.TapNowCanvasNode
	TapNowImportIssue      = canvasimport.TapNowImportIssue
	TapNowImportMetadata   = canvasimport.TapNowImportMetadata
	TapNowImportWarning    = canvasimport.TapNowImportWarning
)

type canvasImportHost struct{ svc *Service }

func (h canvasImportHost) RequireAdmin(user *model.User) error { return h.svc.RequireAdmin(user) }
func (h canvasImportHost) EncryptSecret(value string) (string, error) {
	return h.svc.encryptSettingSecret(value)
}
func (h canvasImportHost) DecryptSecret(value string) (string, error) {
	return h.svc.decryptSettingSecret(value)
}
func (h canvasImportHost) BrandName() string { return h.svc.appearanceBrandName() }
func (s *Service) canvasImports() *canvasimport.Service {
	return canvasimport.New(s.repo, canvasImportHost{svc: s})
}

func (s *Service) AdminLibTVSetting(actor *model.User) (*PublicLibTVSetting, error) {
	return s.canvasImports().AdminLibTVSetting(actor)
}

func (s *Service) UpdateLibTVSetting(actor *model.User, req LibTVSettingRequest) (*PublicLibTVSetting, error) {
	return s.canvasImports().UpdateLibTVSetting(actor, req)
}

func (s *Service) TestLibTV(actor *model.User, projectUUID string) error {
	return s.canvasImports().TestLibTV(actor, projectUUID)
}

func (s *Service) ImportLibTV(userID, canvasProjectID, projectUUID string) (*LibTVImportResult, error) {
	return s.canvasImports().ImportLibTV(userID, canvasProjectID, projectUUID)
}

func (s *Service) ImportTapNow(userID, canvasProjectID, shareID string) (*TapNowImportResult, error) {
	return s.canvasImports().ImportTapNow(userID, canvasProjectID, shareID)
}

func randomToken() string {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return newID() + newID()
	}
	return hex.EncodeToString(value[:])
}
