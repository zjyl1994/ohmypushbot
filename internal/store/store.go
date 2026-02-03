package store

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type PushToken int64

func (pt PushToken) String() string {
	return strconv.FormatInt(int64(pt), 10)
}

func (pt PushToken) Value() (driver.Value, error) {
	return int64(pt), nil
}

func (pt *PushToken) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	v, ok := value.(int64)
	if !ok {
		return fmt.Errorf("unsupported type: %T", value)
	}
	*pt = PushToken(v)
	return nil
}

func ParsePushToken(s string) (PushToken, error) {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, err
	}
	if v <= 0 {
		return 0, errors.New("token must be positive")
	}
	return PushToken(v), nil
}

type Store struct {
	db *gorm.DB
}

type pushToken struct {
	ChatID    int64     `gorm:"column:chat_id;primaryKey"`
	Token     PushToken `gorm:"column:token;uniqueIndex:idx_push_tokens_token"`
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
		return 0, err
	}
	if ok {
		return token, nil
	}
	return s.IssueTokenForce(chatID)
}

func (s *Store) IssueTokenForce(chatID int64) (PushToken, error) {
	now := time.Now().Unix()
	for i := 0; i < 5; i++ {
		token := newToken()
		err := s.upsertToken(chatID, token, now)
		if err == nil {
			return token, nil
		}
		if isUniqueTokenErr(err) {
			continue
		}
		return 0, err
	}
	return 0, errors.New("token collision")
}

func (s *Store) getToken(chatID int64) (PushToken, bool, error) {
	var record pushToken
	err := s.db.First(&record, "chat_id = ?", chatID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
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

func newToken() PushToken {
	return PushToken(rand.Int64N(math.MaxInt64) + 1)
}

func isUniqueTokenErr(err error) bool {
	return errors.Is(err, gorm.ErrDuplicatedKey)
}
