# 🚀 Go-Datastar App
一個基於 Go、Datastar 與 Templ 構建的現代反應式網頁應用。這個專案捨棄了重量級的 JS 框架（如 React/Vue），回歸「以 HTML 為中心」的開發模式，透過 SSE（Server-Sent Events）實現零延遲的即時互動。

## 🛠 技術棧 (The Stack)
* 後端: Go (高效、強型別、原生意義的併發支援)
* 前端框架: Datastar (輕量級超媒體框架，約 11kb)
* 模板引擎: Templ (強型別 HTML-in-Go 組件)
* 樣式: Tailwind CSS (實用優先的 CSS 框架)
* 通訊: SSE (Server-Sent Events) 進行伺服器推送更新

## 📦 快速開始
1. 前置作業
確保你已安裝以下工具：
* Go 1.21+
* Templ CLI (go install ://github.com)
2. 安裝依賴
```bash
go mod tidy
```

3. 生成模板與運行
```bash
# 生成 Templ 代碼
templ generate

# 啟動伺服器
go run cmd/main.go
```

預設訪問地址：http://localhost:8080

## 🏗 專案結構
```text
.
├── cmd/                # 應用程式入口
├── components/         # .templ 組件 (UI 邏輯)
├── internal/           # 業務邏輯與路由處理
├── static/             # 靜態資源 (datastar.js, styles.css)
├── tailwind.config.js  # Tailwind 配置
└── go.mod              # Go 模組定義
```

## 🔥 為什麼選擇此組合？
* 極簡化: 無需前端構建工具（如 Webpack 或 Vite），只需一個 Go 二進位檔。
* 單一來源真理 (Single Source of Truth): 狀態管理保留在 Go 後端，前端僅負責展現與信號傳遞。
* 效能: 透過 Go 的 Goroutines 處理 SSE 串流，能以極低資源消耗支援大量同時在線用戶。
* 開發體驗: Templ 提供的類型檢查讓你告別 HTML 拼寫錯誤。

## 📝 開發備註
* Datastar 信號: 使用 data-model 進行雙向綁定。
* 動態更新: 後端透過 datastar.RenderFragment 推送更新片段，無需重新整理頁面。
