package main

import (
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// What is published cannot be taken back (docs: dev/publish.md), so the pictures a README reaches over the network are fetched
// BEFORE the upload, and each must really be a picture: not a 404, not a page, and not a badge that says it is broken
// (shields.io once answered "retired badge" for every Marketplace badge, and nobody saw it until the page was published).

var remoteImageRe = regexp.MustCompile(`(?:src="|\]\()(https://[^")\s]+)`)

// badWords are the words a badge uses to say it does not work. They are looked for in the text of an SVG.
var badWords = []string{"retired", "invalid", "inaccessible", "not found", "unavailable", "unknown", "error", "failing", "no status", "404", "500", "no longer"}

// remoteImageURLs returns the absolute URLs a README text uses as pictures (Markdown images and src="…"), without links.
func remoteImageURLs(text string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range remoteImageRe.FindAllStringSubmatch(text, -1) {
		u := m[1]
		if isPictureURL(u) && !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	}
	return out
}

// isPictureURL says whether a URL from a src or a link target is a picture (an extension, or a badge service).
func isPictureURL(u string) bool {
	for _, host := range []string{"https://img.shields.io/", "https://flat.badgen.net/"} {
		if strings.HasPrefix(u, host) {
			return true
		}
	}
	path := strings.SplitN(u, "?", 2)[0]
	for _, ext := range []string{".png", ".svg", ".gif", ".jpg", ".jpeg"} {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

// svgText is the words in an SVG, lower-cased.
func svgText(svg string) string {
	text := regexp.MustCompile(`<[^>]*>`).ReplaceAllString(svg, " ")
	return strings.ToLower(strings.Join(strings.Fields(html.UnescapeString(text)), " "))
}

// checkRemoteImage fetches url and says what is wrong with it, or "" when it is a picture that works.
func checkRemoteImage(client *http.Client, url string) string {
	resp, err := client.Get(url)
	if err != nil {
		return err.Error()
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err.Error()
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") {
		return "not a picture (" + ct + ")"
	}
	if strings.HasPrefix(ct, "image/svg") {
		text := svgText(string(body))
		for _, w := range badWords {
			if strings.Contains(text, w) {
				return fmt.Sprintf("the badge says %q", strings.TrimSpace(text))
			}
		}
		if strings.TrimSpace(text) == "" && !strings.Contains(string(body), "<path") && !strings.Contains(string(body), "<rect") {
			return "an empty SVG"
		}
	}
	if len(body) == 0 {
		return "an empty body"
	}
	return ""
}

// checkRemoteImages checks every URL and returns the problems as "url: what", in order. It also returns the badge texts.
func checkRemoteImages(client *http.Client, urls []string) (problems []string) {
	for _, u := range urls {
		if p := checkRemoteImage(client, u); p != "" {
			problems = append(problems, u+": "+p)
		}
	}
	return problems
}

// readmeURLs collects the picture URLs of the README inside the .vsix and of the four READMEs of the repository.
func readmeURLs(root string, vsixReadme string) []string {
	seen := map[string]bool{}
	var all []string
	add := func(text string) {
		for _, u := range remoteImageURLs(text) {
			if !seen[u] {
				seen[u] = true
				all = append(all, u)
			}
		}
	}
	add(vsixReadme)
	for _, f := range []string{"README.md", "README_ja.md", "extension/README.md", "extension/README_ja.md"} {
		if b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f))); err == nil { //nolint:gosec // a README of this repository
			add(string(b))
		}
	}
	sort.Strings(all)
	return all
}

// writePreviewPage writes ui-check-result/publish-check/index.html: every picture the READMEs use, loaded from the network as a
// browser will, so that a person sees what the published page will show before anything is uploaded.
func writePreviewPage(root string, urls []string, problems []string) (string, error) {
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="ja"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>公開前の画像の確認</title>
<style>body{font-family:system-ui,sans-serif;margin:24px;max-width:1100px}img{max-width:100%;border:1px solid #8884;margin:4px 0}li{margin:14px 0}code{word-break:break-all}.ng{color:#c00;font-weight:bold}.ok{color:#080}</style></head><body>
<h1>公開前の画像の確認</h1>
<p>READMEが使う画像を、いま、ネットワークから読み込んで並べています。<b>全部、きちんと絵が出て、バッジに「retired」「invalid」「error」などの文字が無い</b>ことが、アップロードしてよい条件です。</p>
`)
	if len(problems) == 0 {
		b.WriteString(`<p class="ok">機械の検査：すべての画像が取れて、壊れたバッジの文言もありませんでした。</p>`)
	} else {
		b.WriteString(`<p class="ng">機械の検査で問題がありました。アップロードしないでください：</p><ul>`)
		for _, p := range problems {
			fmt.Fprintf(&b, "<li class=\"ng\">%s</li>", html.EscapeString(p))
		}
		b.WriteString("</ul>")
	}
	b.WriteString("<ol>\n")
	for _, u := range urls {
		fmt.Fprintf(&b, "<li><code>%s</code><br><img src=\"%s\" alt=\"\"></li>\n", html.EscapeString(u), html.EscapeString(u))
	}
	b.WriteString("</ol></body></html>\n")
	dir := filepath.Join(root, "ui-check-result", "publish-check")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	page := filepath.Join(dir, "index.html")
	return page, os.WriteFile(page, []byte(b.String()), 0o600)
}

var fetchClient = &http.Client{Timeout: 30 * time.Second}
