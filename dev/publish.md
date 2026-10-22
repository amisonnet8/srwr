# 公開の手順（人間がする）

AI は公開しない（`git push`・タグの push・Marketplace へのアップロードは人間。Release は `release.yml` が goreleaser で作って公開し、AI は `gh release create` を使わない）。AI が用意するのは、`qsoku snapshot`（タグなしで同じ成果物を `dist/` と `bin/vsix/` に作る）、`qsoku dist-try`（入れて開く確認）、`qsoku publish-check`（アップロード前の検査）まで。

## 1. 版を決める

- srwr の版は、**タグ**（`v0.1.0` の形）。Release のバイナリは goreleaser が `-X main.version=<タグ>` で版を埋め、`go install …@v0.1.0` で入れたものはモジュールの版を出すので、どちらも `srwr --version` が `v0.1.0` になる。`qsoku snapshot` は直近のタグを使うので、バイナリはそのタグ（`v0.2.0`）、配布物の名前は次の版の `-snapshot`（`0.2.1-snapshot`）になる
- 拡張の版は `extension/package.json` の `version`で、**srwr の版と同じにする**（タグ `v0.3.0` なら `0.3.0`。拡張が変わらない版でも上げる）。`package-lock.json` の2か所も直して、タグを打つ前にコミットする。違うままタグを push すると、`release.yml` が最初に止まる。Marketplace は**同じ版を2回上げられない**ので、この決まりでも版は必ず進む
- 拡張の README に書く `protocolVersion` が、今の表示サーバーのものと同じか確かめる（`extension/README.md` の「Prerequisites」）

## 2. 公開前の確認（手元）

**公開したものは戻せない**（公開した版は書き換えられない。版を消しても、その番号は使えない。拡張を Remove すると名前が永久に使えなくなる）。だから、操作の前に、機械で確かめられることを全部確かめ、結果を見てから進める（`.claude/rules/distribution.md`）。

1. main に push して、CI が通っていること（`dist` のジョブを含む）
2. `qsoku dist-try` を動かし（中で `qsoku snapshot` が動く）、できたページ（`ui-check-result/dist-try/index.html`）の手順をやる（VSCode と Vim）
3. `qsoku publish-check`（`.vsix` の中身、README がリポジトリと同じこと、**README が使う画像・バッジの URL を全部ネットワークから取って壊れていないこと**を検査し、結果のページ `ui-check-result/publish-check/index.html` を出す）。**ページを開いて、画像が全部出ていて、バッジに「retired」「error」などの文字が無いことを目で見る**。1 つでも変なら上げない

## 3. GitHub Releases（バイナリと .vsix）

1. main のコミットにタグを付けて push する：`git tag v0.1.0 && git push origin v0.1.0`
2. タグの push で `release.yml` が動く。`ci.yml` の検査（Go・拡張・Vim・配布物、3つの OS）をすべて通してから、goreleaser（`.goreleaser.yaml`）が6つの圧縮ファイル・`checksums.txt`・`.vsix` を作って下書きに付け、最後に**公開する**（数分）。人間がするのはタグの push だけ。*Actions* の `Release` が緑で、*Releases* に公開された Release があり、*Assets* に9個あること（本文は goreleaser が前のタグからのコミットで作る）
3. 公開された中身を確かめる：付いたものを1つ取り出して `sha256sum -c checksums.txt`（手元の OS のものだけでよい）。`srwr --version` が `v0.1.0` を言う
4. `go install github.com/amisonnet8/srwr/cmd/srwr@latest` で入ることを確かめる（タグが push されていれば入る）

`release.yml` は、タグを push したときに初めて動く。中身は、タグを打つ前に `qsoku snapshot` で手元で確かめる（同じ goreleaser の設定）。途中で落ちると下書きだけが残り、公開された Release に配布物が欠けることはない。落ちたら、*Actions* のログを AI に見せる。コードを直したら、版を上げて新しいタグを push する。通信などの一時的な失敗なら、*Re-run jobs* で動かし直せる（残った下書きを置き換える）。**公開済みの Release があるタグでは、何も変えずに失敗する**（出し直しは新しいタグで）。

## 4. VSCode Marketplace（Web サイトからアップロード）

コマンドでは公開しない（`vsce publish` も Actions も使わない）。`.vsix` を Web サイトに上げる。

1. main に push しておく。**README の画像は main の URL から読まれる**（`https://github.com/amisonnet8/srwr/raw/main/extension/media/readme/…`）
2. https://marketplace.visualstudio.com/manage で、Publisher `amisonnet8` を作る（無ければ）。Marketplace で `srwr-view` の名前が空いているかも見る
3. 公開に使う `.vsix` は、**Release の *Assets* に付いた `srwr-view-<版>.vsix`**（`release.yml` がタグのコミットから作ったもの）。ブラウザで Release のページからダウンロードする。手元の `bin/vsix/` のものは、作った時点の作業ツリーによって中身が変わりうるので、公開には使わない。ダウンロードしたものと `checksums.txt` が合うことは、AI が確かめられる。`qsoku publish-check <ファイル>` で検査もできる（devcontainer の中に置いたファイルを指す）
4. *New extension* → *Visual Studio Code* で `.vsix` を選ぶ。更新は、拡張の *…* → *Update*
5. 検証が終わるのを待ち、拡張のページを見る：アイコン、README の画像、バッジ（版・評価は、公開されて初めて出る）、「Prerequisites」のリンク
6. 公開後に、インストール数・評価のバッジを README に足し直すかを決める（今は外してある。ルートと拡張の README の英語・日本語の4つ）。使うのは `https://flat.badgen.net/vs-marketplace/i/amisonnet8.srwr-view`（インストール数）と `…/rating/…`（評価。評価が1件もないあいだは 500 になる）。shields.io の Marketplace のバッジは廃止されている
7. 拡張の README だけを直して公開し直すときも、srwr と一緒に版を上げて（0.3.0 → 0.3.1）、タグを打つ（上の3章）。Marketplace の README は `.vsix` の中のものなので、リポジトリの README を直しても、上げ直すまで公開側は変わらない。上げるのは、その Release に付いた `.vsix`

## 5. Open VSX（任意）

VSCodium などは Open VSX を見る。必要になったら、同じ `.vsix` を https://open-vsx.org から上げる（このリポジトリでは、まだ決めていない）。
