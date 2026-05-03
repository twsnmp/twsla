# TWSLA テスト拡充とVHS録画の自動化

## 問題定義 (Problem Statement)
各コマンドの信頼性を担保するためのユニットテスト・E2Eテストを導入し、特に `bubbletea` を用いたTUI操作の検証とデモンストレーションのために VHS を用いた MP4 録画を自動化したい。

## 推奨される方向性 (Recommended Direction)
ユニットテスト（ロジック検証）と E2Eテスト（動作・録画検証）を役割分担させ、`Taskfile` を通じてローカルで簡単に録画を実行できる仕組みを構築します。

1.  **ユニットテスト:** 既存の `cmd/*_test.go` を拡充。GitHub Actions で実行。
2.  **E2E/録画テスト:** `vhs` (`.tape` ファイル) を使用。ローカル実行メイン。
3.  **データ管理:** `testlog/syslog.log` を基準データとして整備。
4.  **自動化:** `Taskfile.yaml` に録画用タスクを追加。

## 検証すべき主要な仮定 (Key Assumptions)
- [x] `vhs` の `Sleep` 設定で `bubbletea` のアニメーションや応答速度を安定してキャプチャできるか。
- [x] `testlog/syslog.log` のデータ量が録画時間（デモとして適切か）に合っているか。
- [x] 開発環境への `vhs`, `ffmpeg`, `ttyd` のインストール状況。

## MVP (最小機能) スコープ
- `testlog/syslog.log` の整備。
- 主要コマンド (`anomaly`, `search`, `count`) に対する `.tape` ファイルの作成。
- `Taskfile` への `record` タスク追加（`vhs` を実行して `images/` 以下に動画出力）。
- GitHub Actions で `go test -short ./...` が通る状態の構築。

## やらないこと (Not Doing)
- ブラウザ上の HTML レポート操作の録画（今回はターミナルのみ）。
- GitHub Actions 上での MP4 生成（リソース消費と依存ツールの都合により、ローカル限定とする）。
- 全コマンドの完璧な E2E 網羅（まずは主要な TUI コマンドから優先）。

## 未解決の質問 (Open Questions)
- 録画した MP4 はリポジトリにコミットするか、それともリリース時のみ生成するか？（`images/` 以下にある PNG 群と同期させるイメージか）。

