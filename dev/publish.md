# 公開の手順（人間がする）

AI は公開しない（`git push`・`gh release create`・Marketplace へのアップロードは人間）。AI が用意するのは、`qsoku dist`（成果物）、`qsoku dist-try`（入れて開く確認）、`qsoku publish-check`（アップロード前の検査）まで。

## 1. 版を決める

- srwr の版は、**タグ**（`v0.1.0` の形）。`srwr --version` は、タグのコミットから `go build` したときだけ `v0.1.0` になる（`runtime/debug` の記録）。タグが無いと `v0.0.0-<日時>-<コミット>+dirty`
- 拡張の版は `extension/package.json` の `version`。srwr の版とは別に上げてよい。Marketplace は**同じ版を2回上げられない**ので、更新のたびに上げる
- 拡張の README に書く `protocolVersion` が、今の表示サーバーのものと同じか確かめる（`extension/README.md` の「Prerequisites」）

## 2. 公開前の確認（手元）

1. main に push して、CI が通っていること（`dist` のジョブを含む）
2. `qsoku dist-try` を動かし、できたページ（`ui-check-result/dist-try/index.html`）の手順をやる（VSCode と Vim）
3. `qsoku publish-check`（`.vsix` の中身を検査して、アップロードの手順を出す）

## 3. GitHub Releases（バイナリと .vsix）

1. main のコミットにタグを付けて push する：`git tag v0.1.0 && git push origin v0.1.0`
2. GitHub の *Releases* → *Draft a new release* でそのタグを選び、*Publish release*
3. `release.yml` が動き、6つの圧縮ファイル・`checksums.txt`・`.vsix` を Release に付ける（数分）。付いたら、*Actions* の `Release` が緑で、*Assets* に9個あること
4. 付いたものを1つ取り出して確かめる：`sha256sum -c checksums.txt`（手元の OS のものだけでよい）。`srwr --version` が `v0.1.0` を言う
5. `go install github.com/amisonnet8/srwr/cmd/srwr@latest` で入ることを確かめる（タグが push されていれば入る）

`release.yml` は、最初の Release で初めて動く（手元では動かせない）。落ちたら、*Actions* のログを AI に見せる。

## 4. VSCode Marketplace（Web サイトからアップロード）

コマンドでは公開しない（`vsce publish` も Actions も使わない）。`.vsix` を Web サイトに上げる。

1. main に push しておく。**README の画像は main の URL から読まれる**（`https://github.com/amisonnet8/srwr/raw/main/extension/media/readme/…`）
2. https://marketplace.visualstudio.com/manage で、Publisher `amisonnet8` を作る（無ければ）。Marketplace で `srwr-view` の名前が空いているかも見る
3. 公開に使う `.vsix` は、Release に付いたものと**同じもの**（`dist/srwr-view-<版>.vsix`）。`qsoku publish-check` を通す
4. *New extension* → *Visual Studio Code* で `.vsix` を選ぶ。更新は、拡張の *…* → *Update*
5. 検証が終わるのを待ち、拡張のページを見る：アイコン、README の画像、バッジ（版・評価は、公開されて初めて出る）、「Prerequisites」のリンク
6. 公開後に、インストール数のバッジ（`img.shields.io/visual-studio-marketplace/i/amisonnet8.srwr-view`）を README に足し直すかを決める（今は外してある。ルートと拡張の README の英語・日本語の4つ）

## 5. Open VSX（任意）

VSCodium などは Open VSX を見る。必要になったら、同じ `.vsix` を https://open-vsx.org から上げる（このリポジトリでは、まだ決めていない）。
