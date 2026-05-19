package model

import (
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type legacyTokenWithoutAutoGroupsOverride struct {
	Id                 int    `gorm:"primaryKey"`
	UserId             int    `gorm:"index"`
	Key                string `gorm:"column:key;type:char(48);uniqueIndex"`
	Status             int    `gorm:"default:1"`
	Name               string `gorm:"index"`
	CreatedTime        int64  `gorm:"bigint"`
	AccessedTime       int64  `gorm:"bigint"`
	ExpiredTime        int64  `gorm:"bigint;default:-1"`
	RemainQuota        int    `gorm:"default:0"`
	UnlimitedQuota     bool
	ModelLimitsEnabled bool
	ModelLimits        string  `gorm:"type:text"`
	AllowIps           *string `gorm:"default:''"`
	UsedQuota          int     `gorm:"default:0"`
	Group              string  `gorm:"column:group;default:''"`
	CrossGroupRetry    bool
	DeletedAt          gorm.DeletedAt `gorm:"index"`
}

func (legacyTokenWithoutAutoGroupsOverride) TableName() string {
	return "tokens"
}

type sqliteTokenColumnInfo struct {
	Name string `gorm:"column:name"`
	Type string `gorm:"column:type"`
}

func openTokenModelTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	DB = db
	LOG_DB = db

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func sqliteTokenColumnNames(t *testing.T, db *gorm.DB) []string {
	t.Helper()

	var columns []sqliteTokenColumnInfo
	if err := db.Raw("PRAGMA table_info(tokens)").Scan(&columns).Error; err != nil {
		t.Fatalf("failed to inspect tokens schema: %v", err)
	}
	names := make([]string, 0, len(columns))
	for _, column := range columns {
		names = append(names, column.Name)
	}
	return names
}

func sqliteTokenColumnType(t *testing.T, db *gorm.DB, columnName string) string {
	t.Helper()

	var columns []sqliteTokenColumnInfo
	if err := db.Raw("PRAGMA table_info(tokens)").Scan(&columns).Error; err != nil {
		t.Fatalf("failed to inspect tokens schema: %v", err)
	}
	for _, column := range columns {
		if column.Name == columnName {
			return strings.ToLower(column.Type)
		}
	}
	t.Fatalf("column %s not found", columnName)
	return ""
}

func TestEnsureTokenAutoGroupsOverrideColumnSQLite(t *testing.T) {
	db := openTokenModelTestDB(t)
	if err := db.AutoMigrate(&legacyTokenWithoutAutoGroupsOverride{}); err != nil {
		t.Fatalf("failed to create legacy tokens table: %v", err)
	}
	if slices.Contains(sqliteTokenColumnNames(t, db), "auto_groups_override") {
		t.Fatal("legacy schema unexpectedly already contains auto_groups_override")
	}

	if err := ensureTokenAutoGroupsOverrideColumn(); err != nil {
		t.Fatalf("failed to run sqlite add-column helper: %v", err)
	}
	if !slices.Contains(sqliteTokenColumnNames(t, db), "auto_groups_override") {
		t.Fatal("expected sqlite helper to add auto_groups_override column")
	}
	if got := sqliteTokenColumnType(t, db, "auto_groups_override"); got != "text" {
		t.Fatalf("unexpected sqlite auto_groups_override type: %s", got)
	}

	if err := ensureTokenAutoGroupsOverrideColumn(); err != nil {
		t.Fatalf("expected sqlite add-column helper to be idempotent, got: %v", err)
	}
}

func TestTokenAutoGroupsOverrideUpdateRoundTrip(t *testing.T) {
	db := openTokenModelTestDB(t)
	if err := db.AutoMigrate(&Token{}); err != nil {
		t.Fatalf("failed to migrate token table: %v", err)
	}
	token := &Token{
		UserId:         1,
		Name:           "update-auto-groups",
		Key:            "sk-test-update-auto-groups",
		Status:         common.TokenStatusEnabled,
		CreatedTime:    1,
		AccessedTime:   1,
		ExpiredTime:    -1,
		RemainQuota:    100,
		UnlimitedQuota: true,
		Group:          "auto",
	}
	if err := db.Create(token).Error; err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	groups := TokenAutoGroups([]string{"default", "", "vip", "default"})
	token.AutoGroupsOverride = &groups
	if err := token.Update(); err != nil {
		t.Fatalf("failed to update token auto groups override: %v", err)
	}

	reloaded, err := GetTokenById(token.Id)
	if err != nil {
		t.Fatalf("failed to reload token: %v", err)
	}
	got := reloaded.GetAutoGroupsOverride()
	if len(got) != 2 || got[0] != "default" || got[1] != "vip" {
		t.Fatalf("unexpected updated auto groups override: %#v", got)
	}

	var raw sql.NullString
	if err := db.Raw("SELECT auto_groups_override FROM tokens WHERE id = ?", token.Id).Scan(&raw).Error; err != nil {
		t.Fatalf("failed to inspect persisted auto_groups_override: %v", err)
	}
	if !raw.Valid || raw.String != `["default","vip"]` {
		t.Fatalf("unexpected persisted auto_groups_override after update: %#v", raw)
	}
}
