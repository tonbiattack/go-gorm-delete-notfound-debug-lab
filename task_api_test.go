package taskapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newRepositoryForTest(t *testing.T) *Repository {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("テスト用DBを開けません: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("テスト用DB接続を取得できません: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	sqlDB.SetMaxOpenConns(1)

	if err := db.AutoMigrate(&Task{}); err != nil {
		t.Fatalf("テスト用テーブルを作成できません: %v", err)
	}
	return NewRepository(db)
}

func TestDeleteTask_ExistingTaskReturnsNoContentAndRemovesIt(t *testing.T) {
	repository := newRepositoryForTest(t)
	if err := repository.Create(context.Background(), Task{ID: "task-1", Title: "削除対象"}); err != nil {
		t.Fatalf("初期データを作成できません: %v", err)
	}

	request := httptest.NewRequest(http.MethodDelete, "/tasks/task-1", nil)
	response := httptest.NewRecorder()
	NewRouter(repository).ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("HTTPステータスが不正です: want=%d got=%d", http.StatusNoContent, response.Code)
	}

	count, err := repository.CountByID(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("削除後の件数を確認できません: %v", err)
	}
	if count != 0 {
		t.Fatalf("削除後の件数が不正です: want=0 got=%d", count)
	}
}

func TestDeleteTask_UnknownIDReturnsNotFoundAndKeepsExistingTask(t *testing.T) {
	repository := newRepositoryForTest(t)
	if err := repository.Create(context.Background(), Task{ID: "task-kept", Title: "保持対象"}); err != nil {
		t.Fatalf("初期データを作成できません: %v", err)
	}

	request := httptest.NewRequest(http.MethodDelete, "/tasks/task-missing", nil)
	response := httptest.NewRecorder()
	NewRouter(repository).ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Errorf("存在しないIDへの削除応答が不正です: want=%d got=%d", http.StatusNotFound, response.Code)
	}

	missingCount, err := repository.CountByID(context.Background(), "task-missing")
	if err != nil {
		t.Fatalf("削除対象IDの件数を確認できません: %v", err)
	}
	if missingCount != 0 {
		t.Errorf("存在しないIDの件数が不正です: want=0 got=%d", missingCount)
	}

	keptCount, err := repository.CountByID(context.Background(), "task-kept")
	if err != nil {
		t.Fatalf("保持対象IDの件数を確認できません: %v", err)
	}
	if keptCount != 1 {
		t.Errorf("保持対象の件数が不正です: want=1 got=%d", keptCount)
	}
}
