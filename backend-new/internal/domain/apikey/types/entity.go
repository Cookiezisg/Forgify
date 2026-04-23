// Package model defines the data shapes for the apikey domain: the
// APIKey entity, value objects (Credentials, ListFilter), enumeration
// constants, and sentinel errors.
//
// Package model 定义 apikey domain 的数据结构：APIKey 实体、值对象
// （Credentials、ListFilter）、枚举常量、sentinel 错误。
package types

import (
	"time"

	"gorm.io/gorm"
)

// APIKey is a user's credential for one LLM provider. KeyEncrypted carries
// the ciphertext (v1:...); KeyMasked is a display string like
// "sk-proj...abc4".
//
// APIKey 是一个用户在某 provider 下的凭证。KeyEncrypted 存密文（v1:...）；
// KeyMasked 是展示字符串如 "sk-proj...abc4"。
type APIKey struct {
	ID           string         `gorm:"primaryKey;type:text" json:"id"`
	UserID       string         `gorm:"not null;index:idx_api_keys_user_id;type:text" json:"userId"`
	Provider     string         `gorm:"not null;index:idx_api_keys_user_provider,priority:2;type:text" json:"provider"`
	DisplayName  string         `gorm:"not null;type:text;default:''" json:"displayName"`
	KeyEncrypted string         `gorm:"not null;type:text" json:"-"`
	KeyMasked    string         `gorm:"not null;type:text" json:"keyMasked"`
	BaseURL      string         `gorm:"type:text;default:''" json:"baseUrl"`
	APIFormat    string         `gorm:"type:text;default:''" json:"apiFormat"`
	TestStatus   string         `gorm:"type:text;default:'pending'" json:"testStatus"`
	TestError    string         `gorm:"type:text;default:''" json:"testError"`
	LastTestedAt *time.Time     `json:"lastTestedAt"`
	CreatedAt    time.Time      `json:"createdAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName locks the DB table to "api_keys" regardless of GORM's default
// pluralization.
//
// TableName 把表名锁定为 "api_keys"，不随 GORM 默认复数化。
func (APIKey) TableName() string { return "api_keys" }
