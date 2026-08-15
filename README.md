# GORMの削除件数未確認で存在しないIDにも204を返すバグを再現して直す

このリポジトリは、GoのHTTP APIで `DELETE /tasks/{id}` を実装するとき、GORMの `Delete` 実行後に削除件数を確認しないと、存在しないIDに対しても `204 No Content` を返してしまう問題を再現する最小プロジェクトです。

この記事用のサンプルでは、存在しないタスクを削除しようとした場合は `404 Not Found` を返すことをAPI契約とします。`DELETE` の応答を常に `204` とする設計もあり得るため、この実装をすべてのAPIに適用するものではありません。重要なのは、削除対象が存在しない場合の扱いを明示し、その契約をテストで固定することです。

## 扱う不具合

| 項目 | バグ状態 | 修正後 |
| --- | --- | --- |
| `DELETE /tasks/task-missing` | `204 No Content` | `404 Not Found` |
| `task-missing` のDB上の件数 | 0件 | 0件 |
| 別のタスクのDB上の件数 | 1件 | 1件 |
| 判断材料 | `result.Error` のみ | `result.Error` と `result.RowsAffected` |

GORMの `Delete` は実行結果を返します。SQL実行自体が成功しても、条件に一致する行が0件であれば、アプリケーション側で `RowsAffected` を見なければ「対象が存在しない」という業務上の状態を区別できません。[GORMの削除ドキュメント](https://gorm.io/docs/delete.html)

## 構成

| パス | 役割 |
| --- | --- |
| `task_api.go` | `net/http` とGORMで構成した最小の削除API |
| `task_api_test.go` | HTTP応答とDB再読込を確認する回帰テスト |
| `evidence/bug-test-output.txt` | 修正前コミットでの意図した失敗出力 |
| `evidence/fixed-test-output.txt` | 修正後の再現テストの成功出力 |
| `evidence/full-test-output.txt` | 全テストの成功出力 |
| `docs/debugging-record.md` | 調査、原因、修正、制約の記録 |

## 前提条件

Go 1.22以上と、CGOを有効にできるCコンパイラが必要です。SQLiteドライバとして `github.com/mattn/go-sqlite3` を利用しています。

## 再現手順

修正前のバグ再現コミットでは、テストが期待した理由で失敗します。

```bash
git checkout 6e61bdb
CGO_ENABLED=1 go test ./... -run '^TestDeleteTask_UnknownIDReturnsNotFoundAndKeepsExistingTask$' -count=1 -v
```

`main` では同じテストと全テストが成功します。

```bash
git checkout main
CGO_ENABLED=1 go test ./... -run '^TestDeleteTask_UnknownIDReturnsNotFoundAndKeepsExistingTask$' -count=1 -v
CGO_ENABLED=1 go test ./... -count=1 -v
```

## 修正の要点

修正前は `Delete` が返す `Error` だけを確認していました。修正後は `RowsAffected == 0` を `ErrTaskNotFound` に変換し、HTTPハンドラーで `404 Not Found` として返します。既存レコードを削除する場合の `204 No Content` と、別レコードが保持されることも同じテストスイートで確認します。

## 制約

このサンプルは物理削除のみを扱います。GORMのソフトデリート、認可、同時削除、削除要求の冪等性方針、監査ログは扱いません。これらを導入する場合は、削除済みレコードを「存在しない」と扱うかを含め、別途API契約を定義してください。
