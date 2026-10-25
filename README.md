# epubify

把 `.txt` 與 `.md` 檔案轉換成 EPUB 3 電子書的命令列工具，以 Go 撰寫。

- 一行指令產生標準 EPUB 3，通過官方 [EPUBCheck](https://github.com/w3c/epubcheck) 驗證，Apple Books、Kobo、Google Play 圖書、Calibre 等閱讀器都能開啟
- 自動產生目錄：TXT 依章節標題分章，Markdown 依標題分章
- 對中文友善：自動辨識 Big5／GBK 編碼、認得「第一章」「卷三」「楔子」這類標題、段首縮排兩字、可選直排
- 單一執行檔，不需要另外安裝 Calibre 或 Pandoc；支援 Windows、macOS、Linux

## 安裝

到 [Releases](https://github.com/elvis3434/epubify/releases) 下載對應平台的執行檔，或用 Go 安裝：

```bash
go install github.com/elvis3434/epubify/cmd/epubify@latest
```

## 使用方式

```bash
epubify novel.txt                                     # 產生 novel.epub
epubify -t "書名" -a "作者" --cover cover.jpg novel.txt
epubify --vertical novel.txt                          # 直排、由右往左翻頁
epubify --encoding big5 old-novel.txt                 # 指定編碼
epubify --chapter-regex '^卷.+' novel.txt              # 自訂章節標題規則
epubify -o notes.epub notes/                          # 整個資料夾合成一本
epubify -o book.epub intro.md ch1.md ch2.md           # 多個檔案依序合成一本
```

選項可以放在檔名前面或後面。沒有指定 `-o` 時，檔案輸入會在原檔旁邊產生同名的 `.epub`，資料夾輸入則在目前目錄產生「資料夾名稱.epub」。

| 選項 | 說明 |
|---|---|
| `-o`, `--output FILE` | 輸出檔名 |
| `-t`, `--title TEXT` | 書名 |
| `-a`, `--author NAME` | 作者，多位作者就重複這個選項 |
| `-l`, `--lang TAG` | 語言標籤，預設 `zh-Hant` |
| `--cover FILE` | 封面圖片（JPEG、PNG、GIF、WebP、SVG） |
| `--vertical` | 直排（vertical-rl），由右往左翻頁 |
| `--css FILE` | 額外的 CSS，附加在預設樣式之後 |
| `--encoding NAME` | 輸入檔編碼：`auto`（預設）、`utf-8`、`big5`、`gbk`、`gb18030`、`shift_jis` 等 |
| `--chapter-regex RE` | 比對 TXT 章節標題的正規表示式 |
| `--paragraph MODE` | TXT 分段方式：`auto`（預設）、`line`、`blank` |
| `--publisher TEXT` | 出版者 |
| `--description TEXT` | 簡介 |
| `--id TEXT` | 書籍唯一識別碼，預設由書名與作者產生 |
| `-q`, `--quiet` | 不顯示提示與警告 |
| `-v`, `--version` | 顯示版本 |

輸入可以是檔案、資料夾或萬用字元（例如 `chapters/*.md`，Windows 的命令列不會自動展開，epubify 會自己處理）。資料夾內的檔案依「自然順序」排列，`2.md` 會排在 `10.md` 前面；以 `.` 開頭的隱藏檔不會被收進來。

## TXT 的處理方式

**章節標題**：每一行去掉前後空白後，符合以下格式、長度在 40 字以內、且不是以「。，、；」結尾的行，會被當成章節標題：

- `第一章`、`第12回`、`第１２３節`、`第一百零八話`（章、節、回、話、幕）
- `第一卷`、`卷三`、`第二部`、`第一集`、`第一篇`：分卷標題
- `序章`、`序`、`楔子`、`引子`、`前言`、`後記`、`尾聲`、`終章`、`番外`
- `Chapter 7`、`CHAPTER IV`、`Part 2`、`Prologue`、`Epilogue`

如果同時有分卷和章節，目錄會變成兩層：章節排在所屬的卷底下。格式特殊的話，可以用 `--chapter-regex` 自訂規則，例如 `--chapter-regex '^== .+ ==$'`。

**其他規則**：

- 第一個標題之前的文字（書名、作者、簡介）會成為獨立的一章。
- 開頭幾行如果有 `《書名》`、`書名：…`、`作者：…`，會自動當成書名與作者（命令列指定的優先）。
- 檔案開頭抄錄的目錄（連續好幾行只有標題、沒有內文，之後又重複出現）會被移除，連同「目錄」那一行。
- 整份文字都找不到標題時，會每約一萬字切成一個「第 N 部分」，避免單一章節太大讓閱讀器變慢。

**分段**：`auto` 會自動判斷。大多數中文小說是一行一段（`line`）；每行固定寬度換行的文字則把相鄰的行合併成一段，以空行或行首縮排分隔（`blank`）。合併時，中文字之間不會多出空白。

## Markdown 的處理方式

檔案開頭可以用 YAML front matter 寫書籍資訊，就不必每次輸入參數：

```markdown
---
title: 我的筆記
author:
  - 作者甲
  - 作者乙
lang: zh-Hant
cover: images/cover.jpg
description: 一句話簡介
publisher: 出版者
date: 2026-10-25
---

# 第一章
```

- 文件以最上層的標題分章：有 `#` 就以 `#` 分章，`##` 成為目錄的第二層；只有 `##` 就以 `##` 分章，以此類推。
- 支援 GFM 語法：表格、刪除線、工作清單、自動連結、註腳。中文段落中的換行不會產生多餘的空白。
- 註腳會放在第一次引用它的那一章結尾；跨章節引用也能正確連結，閱讀器支援的話會以彈出視窗顯示。
- 本機圖片（相對於 Markdown 檔的路徑）會打包進 EPUB。網路圖片不能放進 EPUB，會改成顯示替代文字。
- 連到其他本機檔案的連結會保留文字、移除連結；網址、`mailto:` 與頁內錨點連結會保留。
- Markdown 裡的原始 HTML 不會輸出，避免一段格式錯誤的 HTML 讓整本書無法開啟。

## 書籍資訊的優先順序

命令列選項 > 輸入檔中最先出現的資訊（Markdown 的 front matter，或 TXT 開頭偵測到的書名、作者）> 第一個輸入的檔名（或資料夾名稱）。

書籍識別碼預設由書名與作者產生（UUID v5），所以重新轉換同一本書時識別碼不變，閱讀器會把它當成同一本書的更新版。設定環境變數 [`SOURCE_DATE_EPOCH`](https://reproducible-builds.org/specs/source-date-epoch/) 後，相同的輸入會產生完全相同的檔案。

## 當成 Go 函式庫使用

```go
import "github.com/elvis3434/epubify"

sum, err := epubify.ConvertFile("novel.epub", []string{"novel.txt"}, epubify.Options{
	Authors:  []string{"作者"},
	Vertical: true,
})
```

`epubify.Convert` 則把結果寫到任意 `io.Writer`。

## 目前不支援

PDF、DOCX 輸入；MOBI、AZW3 輸出（Kindle 可以用「Send to Kindle」直接傳 EPUB）；下載網路圖片；LaTeX 數學式；繁簡轉換；內嵌字型；Markdown 檔案之間的互相連結。

## 開發

```bash
go test ./...                                         # 單元與整合測試
EPUBCHECK_JAR=/path/to/epubcheck.jar go test -run TestEPUBCheck -v .   # 用 EPUBCheck 驗證產出
```

CI 會在 Linux、macOS、Windows 上執行測試，並用 EPUBCheck 驗證產生的書。推送 `v*` 標籤（例如 `v1.0.0`）會自動編譯各平台執行檔並建立 GitHub Release。

```
cmd/epubify/        命令列程式
epubify.go          公開 API：Convert、ConvertFile
internal/book/      中介資料模型：書籍資訊、章節、圖片
internal/textenc/   編碼偵測與轉換
internal/txt/       TXT 分章、分段
internal/markdown/  Markdown 轉 XHTML、分章、圖片、註腳
internal/epub/      EPUB 3 封裝（OPF、nav、NCX、CSS、ZIP）
testdata/           測試用範例
```
