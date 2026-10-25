---
title: Markdown 範例筆記
author:
  - 王大明
  - 陳小華
lang: zh-Hant
description: 測試用的 Markdown 文件。
date: 2026-10-25
cover: images/cover.jpg
---

這段文字在第一個標題之前，會成為獨立的一章。

# 第一章 基本格式

一般段落，含有**粗體**、*斜體*、~~刪除線~~與`行內程式碼`。
中文段落的換行
不會產生多餘的空白。

## 清單與引言

- 項目一
- 項目二
  - 子項目

1. 第一步
2. 第二步

- [x] 已完成
- [ ] 未完成

> 這是一段引言。

## 表格

| 名稱 | 數量 | 價格 |
|:-----|:----:|-----:|
| 蘋果 | 3 | 30 |
| 香蕉 & 芭樂 | 5 | 25 |

# 第二章 程式碼與圖片

```go
func main() {
	fmt.Println("Hello, <EPUB> & friends")
}
```

![一張測試圖片](images/pic.png)

![遠端圖片](https://example.com/remote.png)

這裡有註腳[^note]，連到 [Go 官網](https://go.dev)，以及一個[本機連結](other.md)。

---

# 第三章 註腳

再次引用同一個註腳[^note]，以及另一個註腳[^two]。

[^note]: 第一個註腳，在第二章與第三章都被引用。
[^two]: 第二個註腳。
