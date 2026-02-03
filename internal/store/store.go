package store

import (
	"crypto/rand"
	"database/sql/driver"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type PushToken [8]byte

const tokenRetryLimit = 10

func (pt PushToken) String() string {
	return base64.RawURLEncoding.EncodeToString(pt[:])
}

func (pt PushToken) Value() (driver.Value, error) {
	buf := make([]byte, len(pt))
	copy(buf, pt[:])
	return buf, nil
}

func (pt *PushToken) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case []byte:
		if len(v) != len(pt) {
			return fmt.Errorf("invalid token length: %d", len(v))
		}
		copy(pt[:], v)
		return nil
	case string:
		if len(v) != len(pt) {
			return fmt.Errorf("invalid token length: %d", len(v))
		}
		copy(pt[:], []byte(v))
		return nil
	default:
		return fmt.Errorf("unsupported type: %T", value)
	}
}

func ParsePushToken(s string) (PushToken, error) {
	var token PushToken
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return token, err
	}
	if len(raw) != len(token) {
		return token, errors.New("invalid token length")
	}
	copy(token[:], raw)
	return token, nil
}

type Store struct {
	db *gorm.DB
}

type pushToken struct {
	ChatID    int64     `gorm:"column:chat_id;primaryKey"`
	Token     PushToken `gorm:"column:token;type:blob;size:8;uniqueIndex:idx_push_tokens_token"`
	CreatedAt int64     `gorm:"column:created_at"`
}

func (pushToken) TableName() string {
	return "push_tokens"
}

func Open(path string, log *slog.Logger, debug bool) (*Store, error) {
	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}

	if log == nil {
		log = slog.Default()
	}

	gormLogger := NewGormLogger(log)
	if debug {
		gormLogger.LogLevel = logger.Info
	}

	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger:         gormLogger,
		TranslateError: true,
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

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	raw, err := s.db.DB()
	if err != nil {
		return err
	}
	return raw.Close()
}

func (s *Store) GetOrCreateToken(chatID int64) (PushToken, error) {
	token, ok, err := s.getToken(chatID)
	if err != nil {
		return PushToken{}, err
	}
	if ok {
		return token, nil
	}
	return s.IssueTokenForce(chatID)
}

func (s *Store) IssueTokenForce(chatID int64) (PushToken, error) {
	now := time.Now().Unix()
	for i := 0; i < tokenRetryLimit; i++ {
		var token PushToken
		if _, err := rand.Read(token[:]); err != nil {
			return PushToken{}, err
		}
		err := s.upsertToken(chatID, token, now)
		if err == nil {
			return token, nil
		}
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			continue
		}
		return PushToken{}, err
	}
	return PushToken{}, errors.New("token collision")
}

func (s *Store) getToken(chatID int64) (PushToken, bool, error) {
	var record pushToken
	err := s.db.First(&record, "chat_id = ?", chatID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return PushToken{}, false, nil
	}
	if err != nil {
		return PushToken{}, false, err
	}
	return record.Token, true, nil
}

func (s *Store) upsertToken(chatID int64, token PushToken, now int64) error {
	record := pushToken{
		ChatID:    chatID,
		Token:     token,
		CreatedAt: now,
	}
	return s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "chat_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"token":      token,
			"created_at": now,
		}),
	}).Create(&record).Error
}

func (s *Store) ResolveChatID(token PushToken) (int64, bool, error) {
	var record pushToken
	err := s.db.First(&record, "token = ?", token).Error
	if err == nil {
		return record.ChatID, true, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, err
	}
	return 0, false, nil
}
