// 用户分组：分组 -> 系统渠道 -> 模型的可见范围，以及分组级折扣、模型级倍率和货币兑换积分比例。
//
// 目录、报价、任务准入、系统渠道中转和账单预留都从 userGroupAccess 读取同一份分组快照；
// 目录只是读路径，真正的拦截在任务准入和账单预留，客户端提交隐藏渠道 ID 也会失败关闭。

package app

import (
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"yingce/backend/internal/model"
)

const (
	userGroupCacheTTL       = 30 * time.Second
	defaultPointsPerCNYSeed = 1 * CreditScale
	defaultPointsPerUSDSeed = 7 * CreditScale
)

// userGroupState 是 Service 内的分组缓存；多实例部署下由短 TTL 收敛。
type userGroupState struct {
	mu       sync.RWMutex
	snapshot *userGroupSnapshot
	at       time.Time
}

type userGroupSnapshot struct {
	byCode map[string]*resolvedUserGroup
}

type resolvedUserGroup struct {
	model.UserGroup
	channels  map[string]bool
	overrides map[string]model.UserGroupModelOverride
}

// userGroupAccess 是一次请求的分组上下文。bypassVisibility 只放开渠道可见性（管理员和内部能力探测），
// 价格仍按 group 计算。
type userGroupAccess struct {
	userID           string
	group            *resolvedUserGroup
	bypassVisibility bool
}

func normalizeUserGroupCode(code string) string {
	value := strings.ToLower(strings.TrimSpace(code))
	if value == "" {
		return model.DefaultUserGroupCode
	}
	return value
}

func normalizePriceCurrency(value string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "", "CREDITS":
		return model.PriceCurrencyCredits, nil
	case model.PriceCurrencyUSD:
		return model.PriceCurrencyUSD, nil
	case model.PriceCurrencyCNY:
		return model.PriceCurrencyCNY, nil
	default:
		return "", BadAuthRequest("计价货币只支持积分、USD 或 CNY")
	}
}

// EnsureDefaultUserGroup 在启动时补齐 default 分组；它默认开放全部系统渠道，旧部署升级后目录不变。
func (s *Service) EnsureDefaultUserGroup() error {
	_, err := s.repo.UserGroupByCode(model.DefaultUserGroupCode)
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	group := &model.UserGroup{
		ID: newID(), Code: model.DefaultUserGroupCode, Name: "默认分组", BadgeStyle: model.UserGroupBadgeNone,
		RatioBasisPoints: 10_000, PointsPerUSD: defaultPointsPerUSDSeed, PointsPerCNY: defaultPointsPerCNYSeed,
		AllChannels: true, ModelOverridesJSON: "[]",
	}
	if err := s.repo.SaveUserGroupWithChannels(group, nil, true); err != nil {
		return err
	}
	s.invalidateUserGroups()
	return nil
}

func (s *Service) invalidateUserGroups() {
	s.userGroups.mu.Lock()
	s.userGroups.snapshot = nil
	s.userGroups.mu.Unlock()
}

func (s *Service) userGroupSnapshot() (*userGroupSnapshot, error) {
	s.userGroups.mu.RLock()
	if s.userGroups.snapshot != nil && time.Since(s.userGroups.at) < userGroupCacheTTL {
		snapshot := s.userGroups.snapshot
		s.userGroups.mu.RUnlock()
		return snapshot, nil
	}
	s.userGroups.mu.RUnlock()
	groups, err := s.repo.UserGroups()
	if err != nil {
		return nil, err
	}
	bindings, err := s.repo.UserGroupChannels()
	if err != nil {
		return nil, err
	}
	snapshot := &userGroupSnapshot{byCode: make(map[string]*resolvedUserGroup, len(groups))}
	byID := make(map[string]*resolvedUserGroup, len(groups))
	for _, group := range groups {
		overrides, decodeErr := decodeUserGroupOverrides(group.ModelOverridesJSON)
		if decodeErr != nil {
			return nil, decodeErr
		}
		resolved := &resolvedUserGroup{UserGroup: group, channels: map[string]bool{}, overrides: make(map[string]model.UserGroupModelOverride, len(overrides))}
		for _, item := range overrides {
			resolved.overrides[item.ModelKey] = item
		}
		snapshot.byCode[group.Code] = resolved
		byID[group.ID] = resolved
	}
	for _, binding := range bindings {
		if group := byID[binding.GroupID]; group != nil {
			group.channels[binding.ChannelID] = true
		}
	}
	s.userGroups.mu.Lock()
	s.userGroups.snapshot = snapshot
	s.userGroups.at = time.Now()
	s.userGroups.mu.Unlock()
	return snapshot, nil
}

func decodeUserGroupOverrides(raw string) ([]model.UserGroupModelOverride, error) {
	if strings.TrimSpace(raw) == "" {
		return []model.UserGroupModelOverride{}, nil
	}
	var items []model.UserGroupModelOverride
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, errors.New("用户分组模型倍率配置格式无效")
	}
	if items == nil {
		items = []model.UserGroupModelOverride{}
	}
	return items, nil
}

func (s *Service) resolvedUserGroup(code string) (*resolvedUserGroup, error) {
	snapshot, err := s.userGroupSnapshot()
	if err != nil {
		return nil, err
	}
	normalized := normalizeUserGroupCode(code)
	group := snapshot.byCode[normalized]
	if group == nil && normalized == model.DefaultUserGroupCode {
		// 默认分组是计费基线：启动时已补齐，这里兜住新库或被并发清空的窗口。
		if ensureErr := s.EnsureDefaultUserGroup(); ensureErr != nil {
			if _, lookupErr := s.repo.UserGroupByCode(model.DefaultUserGroupCode); lookupErr != nil {
				return nil, ensureErr
			}
		}
		s.invalidateUserGroups()
		if snapshot, err = s.userGroupSnapshot(); err != nil {
			return nil, err
		}
		group = snapshot.byCode[normalized]
	}
	if group == nil {
		// 分组被删除前必须清空用户，所以这里只会是配置损坏；失败关闭，不回退到默认分组。
		return nil, Forbidden("当前账号分组配置无效，请联系管理员")
	}
	return group, nil
}

// userGroupAccessForUser applies the selected catalog group without local role exemptions.
func (s *Service) userGroupAccessForUser(user *model.User) (*userGroupAccess, error) {
	if user == nil {
		return nil, Unauthorized("请先登录")
	}
	group, err := s.resolvedUserGroup(user.UserGroup)
	if err != nil {
		return nil, err
	}
	return &userGroupAccess{userID: user.ID, group: group, bypassVisibility: false}, nil
}

// userGroupAccess 按用户 ID 读取分组上下文；账单和任务准入使用它，用户不存在时失败关闭。
func (s *Service) userGroupAccess(userID string) (*userGroupAccess, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, Unauthorized("请先登录")
	}
	user, err := s.repo.User(userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, Unauthorized("账号不存在或已删除")
	}
	if err != nil {
		return nil, err
	}
	return s.userGroupAccessForUser(user)
}

// systemGroupAccess 只用于不面向具体用户的内部能力探测（模型名称、能力画像）；
// 它放开渠道可见性，价格按默认分组计算，不能用于账单。
func (s *Service) systemGroupAccess() (*userGroupAccess, error) {
	group, err := s.resolvedUserGroup(model.DefaultUserGroupCode)
	if err != nil {
		return nil, err
	}
	return &userGroupAccess{group: group, bypassVisibility: true}, nil
}

func (a *userGroupAccess) allowsChannel(channelID string) bool {
	if a == nil || a.group == nil {
		return false
	}
	if a.bypassVisibility || a.group.AllChannels {
		return true
	}
	return a.group.channels[strings.TrimSpace(channelID)]
}

func (a *userGroupAccess) requireChannel(channelID string) error {
	if a.allowsChannel(channelID) {
		return nil
	}
	return ModelCatalogMismatch("当前账号分组无权使用该模型，请刷新后重新选择")
}

func (a *userGroupAccess) groupCode() string {
	if a == nil || a.group == nil {
		return ""
	}
	return a.group.Code
}

// groupPricing 是一次下单使用的分组价格因子。
type groupPricing struct {
	GroupCode        string
	Currency         string
	RateMicrocredits int64
	MultiplierBPS    int64
}

// pricing 返回模型在当前分组下的倍率和货币兑换比例。modelKeys 按优先级匹配分组模型覆盖项。
func (a *userGroupAccess) pricing(currency string, modelKeys ...string) (groupPricing, error) {
	if a == nil || a.group == nil {
		return groupPricing{}, errors.New("缺少用户分组上下文")
	}
	normalizedCurrency, err := normalizePriceCurrency(currency)
	if err != nil {
		return groupPricing{}, err
	}
	group := a.group
	result := groupPricing{GroupCode: group.Code, Currency: normalizedCurrency, MultiplierBPS: group.RatioBasisPoints}
	var override *model.UserGroupModelOverride
	for _, key := range modelKeys {
		if item, ok := group.overrides[strings.TrimSpace(key)]; ok && strings.TrimSpace(key) != "" {
			override = &item
			break
		}
	}
	if override != nil && override.RatioBasisPoints > 0 {
		result.MultiplierBPS = combineMultiplierBPS(group.RatioBasisPoints, override.RatioBasisPoints)
	}
	switch normalizedCurrency {
	case model.PriceCurrencyCredits:
		result.RateMicrocredits = CreditScale
	case model.PriceCurrencyUSD:
		result.RateMicrocredits = group.PointsPerUSD
		if override != nil && override.PointsPerUSD > 0 {
			result.RateMicrocredits = override.PointsPerUSD
		}
	case model.PriceCurrencyCNY:
		result.RateMicrocredits = group.PointsPerCNY
		if override != nil && override.PointsPerCNY > 0 {
			result.RateMicrocredits = override.PointsPerCNY
		}
	}
	if result.RateMicrocredits <= 0 {
		return groupPricing{}, ModelPriceNotConfigured("当前账号分组尚未配置 " + normalizedCurrency + " 兑换积分比例")
	}
	if result.MultiplierBPS <= 0 {
		return groupPricing{}, ModelPriceNotConfigured("当前账号分组倍率无效")
	}
	return result, nil
}

// convertPrice 把百万分之一货币单位的价格换算成微积分并向上取整；积分计价时原样返回。
func (p groupPricing) convertPrice(price int64) (int64, error) {
	if price < 0 {
		return 0, errors.New("模型价格无效")
	}
	if p.RateMicrocredits == CreditScale || price == 0 {
		return price, nil
	}
	product := new(big.Int).Mul(big.NewInt(price), big.NewInt(p.RateMicrocredits))
	scale := big.NewInt(CreditScale)
	product.Add(product, new(big.Int).Sub(scale, big.NewInt(1)))
	product.Quo(product, scale)
	if !product.IsInt64() {
		return 0, errors.New("模型价格换算溢出")
	}
	return product.Int64(), nil
}

// combineMultiplierBPS 连乘多个基点倍率并向上取整，结果至少为 1 个基点。
func combineMultiplierBPS(values ...int64) int64 {
	result := big.NewInt(10_000)
	scale := big.NewInt(10_000)
	for _, value := range values {
		if value <= 0 {
			continue
		}
		result.Mul(result, big.NewInt(value))
		result.Add(result, big.NewInt(9_999))
		result.Quo(result, scale)
	}
	if !result.IsInt64() || result.Int64() > 1<<40 {
		return 1 << 40
	}
	if result.Int64() < 1 {
		return 1
	}
	return result.Int64()
}

// ModelCatalogForUser 是 HTTP 目录入口：按登录用户的分组过滤渠道并换算价格。
func (s *Service) ModelCatalogForUser(user *model.User, intent *ModelRequestIntent) (*ModelCatalogResponse, error) {
	access, err := s.userGroupAccessForUser(user)
	if err != nil {
		return nil, err
	}
	return s.ModelCatalog(access, intent)
}
