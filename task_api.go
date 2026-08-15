package taskapi

import (
	"context"
	"net/http"
	"strings"

	"gorm.io/gorm"
)

// Task は削除対象となる最小のタスクです。
type Task struct {
	ID    string `gorm:"primaryKey"`
	Title string `gorm:"not null"`
}

// Repository はタスクの永続化を担当します。
type Repository struct {
	db *gorm.DB
}

// NewRepository はテストまたはアプリケーションで利用するリポジトリを生成します。
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Create はタスクを保存します。
func (r *Repository) Create(ctx context.Context, task Task) error {
	return r.db.WithContext(ctx).Create(&task).Error
}

// DeleteByID はIDに一致するタスクを削除します。
// バグ状態では削除件数を確認せず、SQL実行エラーだけを呼び出し元へ返します。
func (r *Repository) DeleteByID(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).Delete(&Task{}, "id = ?", id)
	return result.Error
}

// CountByID は指定IDのタスク件数を返します。
func (r *Repository) CountByID(ctx context.Context, id string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Task{}).Where("id = ?", id).Count(&count).Error
	return count, err
}

// NewRouter はDELETE /tasks/{id}を公開する最小のルーターを返します。
func NewRouter(repository *Repository) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/tasks/", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodDelete {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		id := strings.TrimPrefix(request.URL.Path, "/tasks/")
		if id == "" || strings.Contains(id, "/") {
			writer.WriteHeader(http.StatusNotFound)
			return
		}

		if err := repository.DeleteByID(request.Context(), id); err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}

		writer.WriteHeader(http.StatusNoContent)
	})
	return mux
}
