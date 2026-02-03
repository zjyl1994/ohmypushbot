package main

import (
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type tokenStore struct {
	db *gorm.DB
}

type pushToken struct {
	ChatID    int64  `gorm:"column:chat_id;primaryKey"`
	Token     string `gorm:"column:token;uniqueIndex:idx_push_tokens_token"`
	CreatedAt int64  `gorm:"column:created_at"`
	RevokedAt *int64 `gorm:"column:revoked_at"`
}

func (pushToken) TableName() string {
	return "push_tokens"
}

func openStore(path string) (*tokenStore, error) {
	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
	}

	gormLogger := NewGormLogger(slog.Default())
	if parseEnvBool("DEBUG") {
		gormLogger.LogLevel = logger.Info
	}

	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: gormLogger,
	})
	if err != nil {
		return nil, err
	}

	if err := db.Exec("PRAGMA journal_mode=WAL;").Error; err != nil {
		return nil, err
	}
	if err := db.Exec("PRAGMA synchronous=NORMAL;").Error; err != nil {
		return nil, err
	}
	if err := db.Exec("PRAGMA busy_timeout=5000;").Error; err != nil {
		return nil, err
	}

	if err := db.AutoMigrate(&pushToken{}); err != nil {
		return nil, err
	}

	return &tokenStore{db: db}, nil
}

func (s *tokenStore) Close() error {
	raw, err := s.db.DB()
	if err != nil {
		return err
	}
	return raw.Close()
}

func (s *tokenStore) getOrCreateToken(chatID int64) (string, error) {
	token, revoked, err := s.getToken(chatID)
	if err != nil {
		return "", err
	}
	if token != "" && !revoked {
		return token, nil
	}
	return s.issueTokenForce(chatID)
}

func (s *tokenStore) issueTokenForce(chatID int64) (string, error) {
	now := time.Now().Unix()
	for i := 0; i < 5; i++ {
		token, err := newToken()
		if err != nil {
			return "", err
		}
		err = s.upsertToken(chatID, token, now)
		if err == nil {
			return token, nil
		}
		if isUniqueTokenErr(err) {
			continue
		}
		return "", err
	}
	return "", errors.New("token collision")
}

func (s *tokenStore) getToken(chatID int64) (string, bool, error) {
	var record pushToken
	err := s.db.First(&record, "chat_id = ?", chatID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return record.Token, record.RevokedAt != nil, nil
}

func (s *tokenStore) upsertToken(chatID int64, token string, now int64) error {
	record := pushToken{
		ChatID:    chatID,
		Token:     token,
		CreatedAt: now,
		RevokedAt: nil,
	}
	return s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "chat_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"token":      token,
			"created_at": now,
			"revoked_at": nil,
		}),
	}).Create(&record).Error
}

func (s *tokenStore) revokeToken(chatID int64) (bool, error) {
	now := time.Now().Unix()
	res := s.db.Model(&pushToken{}).
		Where("chat_id = ? AND revoked_at IS NULL", chatID).
		Update("revoked_at", now)
	return res.RowsAffected > 0, res.Error
}

func (s *tokenStore) resolveChatID(token string) (int64, bool, error) {
	var record pushToken
	err := s.db.First(&record, "token = ?", token).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if record.RevokedAt != nil {
		return 0, false, nil
	}
	return record.ChatID, true, nil
}

func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", buf), nil
}

func isUniqueTokenErr(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed: push_tokens.token")
}
