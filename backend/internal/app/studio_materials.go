package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"yingce/backend/internal/model"
)

var studioMaterialID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var studioSHA256 = regexp.MustCompile(`^[a-f0-9]{64}$`)
var studioContentType = regexp.MustCompile(`^[a-z]+/[a-zA-Z0-9.+-]+$`)

type StudioResourceMeta struct {
	Width      int   `json:"width,omitempty"`
	Height     int   `json:"height,omitempty"`
	DurationMs int64 `json:"durationMs,omitempty"`
}
type StudioUploadRequest struct {
	CommandID   string `json:"commandId"`
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
	ByteLength  string `json:"byteLength"`
	SHA256      string `json:"sha256"`
	StudioResourceMeta
}
type StudioAdoptRequest struct {
	AssetID   string `json:"assetId"`
	VersionID string `json:"versionId"`
	StudioResourceMeta
}
type StudioUploadResult struct {
	ID       string                       `json:"id"`
	Upload   platformclient.UploadSession `json:"upload"`
	Resource *model.Resource              `json:"resource,omitempty"`
}

type studioMaterialCall struct {
	s        *Service
	ctx      context.Context
	userID   string
	m        *platformclient.Materials
	response *platformclient.Response
}

func (s *Service) materialCall(ctx context.Context, userID string, session platformclient.SessionContext) *studioMaterialCall {
	return &studioMaterialCall{s: s, ctx: ctx, userID: userID, m: s.studioPlatform.Client.CurrentMaterials(session), response: &platformclient.Response{Header: make(http.Header)}}
}
func (c *studioMaterialCall) collect(r *platformclient.Response) {
	if r != nil {
		r.AppendCookies(c.response.Header)
	}
}

func studioMaterialError(err error) error {
	if err == nil {
		return nil
	}
	var upstream *platformclient.Error
	if errors.As(err, &upstream) {
		switch upstream.StatusCode {
		case 401, 403, 429:
			return platformIdentityError(err)
		case 404:
			return NotFound("素材不存在或不可访问")
		case 400, 409, 413, 422:
			return WrapAppError(upstream.StatusCode, "素材请求无法完成，请检查文件或重新读取状态", err)
		}
	}
	return WrapAppError(503, "素材服务暂时不可用，请稍后查询原状态", err)
}

func validStudioResourceMeta(meta StudioResourceMeta) bool {
	return meta.Width >= 0 && meta.Height >= 0 && meta.DurationMs >= 0
}

func (s *Service) StudioCreateUpload(ctx context.Context, userID string, session platformclient.SessionContext, in StudioUploadRequest) (*StudioUploadResult, *platformclient.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	c := s.materialCall(ctx, userID, session)
	n, err := strconv.ParseInt(in.ByteLength, 10, 64)
	if err != nil || n <= 0 || strconv.FormatInt(n, 10) != in.ByteLength || !studioMaterialID.MatchString(in.CommandID) || !studioSHA256.MatchString(in.SHA256) || !studioContentType.MatchString(in.ContentType) || strings.TrimSpace(in.Filename) == "" || utf8.RuneCountInString(in.Filename) > 255 || !validStudioResourceMeta(in.StudioResourceMeta) {
		return nil, c.response, BadAuthRequest("上传文件信息无效")
	}
	id := uuid.NewSHA1(uuid.NameSpaceURL, []byte("studio-upload\n"+userID+"\n"+in.CommandID)).String()
	body, _ := json.Marshal(in)
	u, err := s.repo.WithContext(ctx).CreateStudioMaterialUpload(model.StudioMaterialUpload{ID: id, UserID: userID, CommandID: "studio-upload-" + id, IntentJSON: string(body)})
	if err != nil {
		return nil, c.response, err
	}
	if u.IntentJSON != string(body) {
		return nil, c.response, NewAppError(409, "该上传命令已用于其他文件")
	}
	var result platformclient.Result[platformclient.UploadSession]
	if u.UploadID != "" {
		result, err = c.m.UploadStatus(ctx, u.UploadID)
	} else {
		result, err = c.m.CreateUpload(ctx, platformclient.UploadCreate{CommandID: u.CommandID, Filename: in.Filename, ContentType: in.ContentType, ByteLength: in.ByteLength, SHA256: in.SHA256})
	}
	c.collect(result.Response)
	if err != nil {
		return nil, c.response, studioMaterialError(err)
	}
	out, err := c.uploadResult(u, result.Data)
	return out, c.response, err
}

func (s *Service) StudioUploadStatus(ctx context.Context, userID string, session platformclient.SessionContext, id string, complete *platformclient.UploadComplete) (*StudioUploadResult, *platformclient.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	c := s.materialCall(ctx, userID, session)
	u, err := s.repo.WithContext(ctx).StudioMaterialUpload(userID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, c.response, NotFound("上传不存在")
	}
	if err != nil {
		return nil, c.response, err
	}
	if u.UploadID == "" {
		return nil, c.response, NewAppError(409, "上传状态待确认，请使用原上传命令恢复")
	}
	var result platformclient.Result[platformclient.UploadSession]
	if complete == nil {
		result, err = c.m.UploadStatus(ctx, u.UploadID)
	} else {
		if len(complete.Parts) == 0 || len(complete.Parts) > 10000 {
			return nil, c.response, BadAuthRequest("上传分片无效")
		}
		seen := map[int]bool{}
		for _, p := range complete.Parts {
			if p.PartNumber < 1 || p.PartNumber > 10000 || seen[p.PartNumber] || p.ETag == "" || len(p.ETag) > 512 {
				return nil, c.response, BadAuthRequest("上传分片无效")
			}
			seen[p.PartNumber] = true
		}
		result, err = c.m.CompleteUpload(ctx, u.UploadID, *complete)
	}
	c.collect(result.Response)
	if err != nil {
		return nil, c.response, studioMaterialError(err)
	}
	out, err := c.uploadResult(u, result.Data)
	return out, c.response, err
}

func (c *studioMaterialCall) uploadResult(u *model.StudioMaterialUpload, result platformclient.UploadSession) (*StudioUploadResult, error) {
	if !studioMaterialID.MatchString(result.UploadID) || result.CommandID != u.CommandID || (u.UploadID != "" && u.UploadID != result.UploadID) {
		return nil, NewAppError(502, "上传身份响应不匹配")
	}
	if err := c.s.repo.WithContext(c.ctx).SetStudioMaterialUploadID(c.userID, u.ID, result.UploadID); err != nil {
		return nil, err
	}
	out := &StudioUploadResult{ID: u.ID, Upload: result}
	var in StudioUploadRequest
	if err := json.Unmarshal([]byte(u.IntentJSON), &in); err != nil {
		return nil, err
	}
	switch result.State {
	case "ready":
		if result.Asset == nil || result.Asset.File.SHA256 != in.SHA256 || result.Asset.File.ByteLength != in.ByteLength || result.Asset.File.ContentType != in.ContentType {
			return nil, NewAppError(502, "上传文件响应不匹配")
		}
		var err error
		out.Resource, err = c.adopt(*result.Asset, in.StudioResourceMeta)
		if err != nil {
			return nil, err
		}
	case "awaiting_upload":
		if result.ByteLength != in.ByteLength || result.PartSize == "" || len(result.Tickets) == 0 {
			return nil, NewAppError(502, "上传票据响应不完整")
		}
	case "verifying", "sealing":
	case "failed", "expired":
	default:
		return nil, NewAppError(502, "上传状态不受支持")
	}
	return out, nil
}

func (s *Service) StudioAdoptMaterial(ctx context.Context, userID string, session platformclient.SessionContext, in StudioAdoptRequest) (*model.Resource, *platformclient.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	c := s.materialCall(ctx, userID, session)
	if !studioMaterialID.MatchString(in.AssetID) || !studioMaterialID.MatchString(in.VersionID) || !validStudioResourceMeta(in.StudioResourceMeta) {
		return nil, c.response, BadAuthRequest("素材引用无效")
	}
	a, err := c.m.Asset(ctx, in.AssetID, in.VersionID)
	c.collect(a.Response)
	if err != nil {
		return nil, c.response, studioMaterialError(err)
	}
	if a.Data.AssetID != in.AssetID || a.Data.Version.VersionID != in.VersionID {
		return nil, c.response, NewAppError(502, "素材版本响应不匹配")
	}
	res, err := c.adopt(a.Data, in.StudioResourceMeta)
	return res, c.response, err
}

func (c *studioMaterialCall) adopt(a platformclient.Asset, meta StudioResourceMeta) (*model.Resource, error) {
	owner, err := strconv.ParseInt(a.Owner.ID, 10, 64)
	if err != nil || owner <= 0 || strconv.FormatInt(owner, 10) != a.Owner.ID || a.Owner.Kind != "account" || uuid.NewSHA1(uuid.NameSpaceURL, []byte(c.s.studioPlatform.Origin()+"\n"+a.Owner.ID)).String() != c.userID {
		return nil, NotFound("素材不存在或不可访问")
	}
	size, err := strconv.ParseInt(a.File.ByteLength, 10, 64)
	if err != nil || size <= 0 || a.State != "ready" || a.File.State != "ready" || !studioMaterialID.MatchString(a.AssetID) || !studioMaterialID.MatchString(a.Version.VersionID) || !studioMaterialID.MatchString(a.File.FileID) || a.Version.AssetID != a.AssetID || a.Version.FileID != a.File.FileID || !studioSHA256.MatchString(a.File.SHA256) {
		return nil, NewAppError(502, "素材文件身份无效")
	}
	id := uuid.NewSHA1(uuid.NameSpaceURL, []byte("studio-material\n"+c.userID+"\n"+a.AssetID+"\n"+a.Version.VersionID)).String()
	kind := a.Kind
	if kind == "document" {
		kind = "file"
	}
	if kind != "image" && kind != "video" && kind != "audio" && kind != "file" {
		return nil, NewAppError(502, "素材类型不受支持")
	}
	now := time.Now()
	ref := model.StudioMaterialReference{ResourceID: id, UserID: c.userID, AssetID: a.AssetID, VersionID: a.Version.VersionID, FileID: a.File.FileID, OwnerID: a.Owner.ID, SHA256: a.File.SHA256, ByteLength: a.File.ByteLength, CreatedAt: now, UpdatedAt: now}
	resource := model.Resource{ID: id, UserID: c.userID, Provider: "platform", Kind: kind, Status: model.ResourceStatusPending, MimeType: a.File.ContentType, Size: size, Width: meta.Width, Height: meta.Height, DurationMs: meta.DurationMs, CreatedAt: now, UpdatedAt: now}
	repo := c.s.repo.WithContext(c.ctx)
	if err := repo.CreateStudioMaterialResource(ref, resource); err != nil {
		return nil, err
	}
	saved, err := repo.ResourceForUser(c.userID, id)
	if err != nil {
		return nil, err
	}
	if saved.Status == model.ResourceStatusReady {
		return saved, nil
	}
	command := "studio-resource-" + id
	b, err := c.m.Binding(c.ctx, command)
	c.collect(b.Response)
	var upstream *platformclient.Error
	if errors.As(err, &upstream) && upstream.StatusCode == 404 {
		b, err = c.m.Bind(c.ctx, a.AssetID, platformclient.BindingRequest{CommandID: command, VersionID: a.Version.VersionID, TargetType: "resource", TargetID: id, Slot: "original"})
		c.collect(b.Response)
	}
	if err != nil {
		return nil, studioMaterialError(err)
	}
	validBinding := func(b platformclient.Binding) bool {
		return studioMaterialID.MatchString(b.BindingID) && b.AssetID == a.AssetID && b.VersionID == a.Version.VersionID && b.TargetApp == "studio" && b.TargetType == "resource" && b.TargetID == id && b.Slot == "original"
	}
	if !validBinding(b.Data) || (b.Data.State != "pending" && b.Data.State != "active") {
		return nil, NewAppError(502, "素材绑定响应不匹配")
	}
	if err := repo.SetStudioMaterialBinding(c.userID, id, b.Data.BindingID); err != nil {
		return nil, err
	}
	if b.Data.State == "pending" {
		b, err = c.m.ConfirmBinding(c.ctx, b.Data.BindingID)
		c.collect(b.Response)
		if err != nil {
			return nil, studioMaterialError(err)
		}
		if !validBinding(b.Data) || b.Data.State != "active" {
			return nil, NewAppError(502, "素材绑定尚未确认")
		}
	}
	if err := repo.ReadyStudioMaterialResource(c.userID, id); err != nil {
		return nil, err
	}
	return repo.ResourceForUser(c.userID, id)
}

func (s *Service) StudioResource(ctx context.Context, userID, id string) (*model.Resource, error) {
	resource, err := s.repo.WithContext(ctx).ResourceForUser(userID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, NotFound("资源不存在或不可访问")
	}
	if err != nil {
		return nil, err
	}
	if resource.Provider != "platform" || resource.Status != model.ResourceStatusReady {
		return nil, NewAppError(409, "素材尚未就绪")
	}
	return resource, nil
}

func (s *Service) StudioMaterialAssets(ctx context.Context, session platformclient.SessionContext, query platformclient.AssetQuery) (platformclient.Page[platformclient.Asset], *platformclient.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	result, err := s.studioPlatform.Client.CurrentMaterials(session).Assets(ctx, query)
	return result.Data, result.Response, studioMaterialError(err)
}
func (s *Service) StudioMaterialStorage(ctx context.Context, session platformclient.SessionContext) (platformclient.Storage, *platformclient.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	result, err := s.studioPlatform.Client.CurrentMaterials(session).Storage(ctx)
	return result.Data, result.Response, studioMaterialError(err)
}
