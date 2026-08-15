# デバッグ記録：存在しないタスクの削除が204になる問題

## 目的

`DELETE /tasks/{id}` に存在しないIDを指定したとき、このサンプルのAPI契約では `404 Not Found` を返します。バグ状態では、GORMの削除処理でSQL実行エラーだけを確認していたため、対象行が0件でも `204 No Content` を返していました。HTTP応答だけでなく、削除対象と無関係のタスクが保持されることをDBへ再読込して確認します。

## 再現条件

| 項目 | 内容 |
| --- | --- |
| バグを含むコミット | `6e61bdb` |
| テスト名 | `TestDeleteTask_UnknownIDReturnsNotFoundAndKeepsExistingTask` |
| 初期状態 | `task-kept` が1件存在し、`task-missing` は存在しない |
| 操作 | `DELETE /tasks/task-missing` |
| 期待する最終状態 | HTTP 404、`task-missing` は0件、`task-kept` は1件 |

## 最初に観測した事実

| 観測対象 | 期待値 | 実際値 | 根拠 |
| --- | --- | --- | --- |
| HTTPステータス | 404 | 204 | 失敗テストの expected-versus-actual 出力 |
| レスポンス本文 | エラーを表す応答 | 204のため本文なし | `httptest.ResponseRecorder` での応答 |
| `task-missing` の最終件数 | 0件 | 0件 | `CountByID` によるDB再読込 |
| `task-kept` の最終件数 | 1件 | 1件 | `CountByID` によるDB再読込 |

```text
存在しないIDへの削除応答が不正です: want=404 got=204
```

削除対象が最初から存在しないため、DBの最終件数が0件であること自体は期待どおりです。ここで問題になるのは、存在しないことをAPI利用者へ伝えるべき契約なのに、HTTP応答が成功を示していた点です。また、保持対象の件数を確認することで、広範な削除ではなく応答判定の問題であることを切り分けました。

## 仮説と切り分け

| 仮説 | 確認方法 | 結果 |
| --- | --- | --- |
| HTTPハンドラーだけが誤って204を返している | ハンドラーとリポジトリの戻り値を読む | 採用。`nil` エラーの後に無条件で204を返していた。 |
| GORMの削除処理が失敗している | `result.Error` を確認し、失敗テストのエラー種別を確認する | 棄却。SQL実行エラーは発生していない。 |
| 条件が失われて別タスクを削除している | `task-kept` をDBから再読込する | 棄却。保持対象は1件のままだった。 |
| 削除対象の不存在を検出する情報がない | GORMの削除結果を確認する | 採用。`RowsAffected` が0件を示すが、バグ状態では捨てられていた。 |

## 原因

バグ状態の `DeleteByID` は `db.Delete` の戻り値から `Error` だけを返していました。`DELETE` 文が正常に実行され、条件に一致する行が0件であってもSQLエラーにはなりません。そのため、リポジトリは `nil` を返し、HTTPハンドラーは無条件で204を返していました。

GORMの `Delete` は削除条件を指定して呼び出せます。[公式の削除ドキュメント](https://gorm.io/docs/delete.html)に基づき、このサンプルでは実行結果の `RowsAffected` を業務契約の判定材料にします。

## 修正

```go
result := r.db.WithContext(ctx).Delete(&Task{}, "id = ?", id)
if result.Error != nil {
    return result.Error
}
if result.RowsAffected == 0 {
    return ErrTaskNotFound
}
return nil
```

リポジトリで0件削除を `ErrTaskNotFound` に変換し、HTTPハンドラーがこのエラーを `404 Not Found` に対応付けます。SQL実行失敗と「対象行がない」状態を混同せずに扱えるようになります。

## 再発防止テスト

| テスト | 守る契約 | 最終観測 |
| --- | --- | --- |
| `TestDeleteTask_UnknownIDReturnsNotFoundAndKeepsExistingTask` | 存在しないIDには404を返す | 不存在IDは0件、保持対象は1件 |
| `TestDeleteTask_ExistingTaskReturnsNoContentAndRemovesIt` | 存在するIDは204で削除する | 削除対象は0件 |

## 再現手順

```bash
# 修正前：意図したHTTPステータス差で失敗する
git checkout 6e61bdb
CGO_ENABLED=1 go test ./... -run '^TestDeleteTask_UnknownIDReturnsNotFoundAndKeepsExistingTask$' -count=1 -v

# 修正後：再現・回帰・全体テストが通る
git checkout main
CGO_ENABLED=1 go test ./... -run '^TestDeleteTask_UnknownIDReturnsNotFoundAndKeepsExistingTask$' -count=1 -v
CGO_ENABLED=1 go test ./... -count=1 -v
```

## 設計上の範囲

この修正は「存在しない削除対象には404を返す」という契約だけを扱います。削除要求を常に成功として扱う冪等なAPI、GORMのソフトデリート、認可、並行削除、監査ログには適用していません。これらを扱う場合は、削除済みのリソースを404と204のどちらにするかをAPI仕様として別途決定し、対応する統合テストを追加します。
