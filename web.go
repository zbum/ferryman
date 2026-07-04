package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"net/http"
	"strconv"
)

// pageTop 은 인코딩/디코딩 폼을 담는 단일 HTML 페이지 뼈대다.
// 외부 CSS/JS/CDN 의존 없이 airgap 에서 그대로 동작한다.
const pageTop = `<!doctype html><html lang="ko"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>ferryman — 텍스트 QR 생성/해독</title>
<style>
body{font-family:system-ui,-apple-system,sans-serif;max-width:820px;margin:2rem auto;padding:0 1rem;color:#111}
h1{font-size:1.3rem} h2{font-size:1.05rem;margin-top:2rem}
textarea{width:100%;height:9rem;font-family:monospace;font-size:.9rem}
button{padding:.5rem 1rem;font-size:1rem;cursor:pointer;margin-top:.5rem}
.grid{display:flex;flex-wrap:wrap;gap:1rem;margin-top:1rem}
.card{border:1px solid #ccc;padding:.5rem;text-align:center}
.card img{display:block;image-rendering:pixelated;width:260px;height:260px}
.card span{font-size:.8rem;color:#555}
label{font-size:.85rem;color:#333}
pre{background:#f4f4f4;padding:1rem;white-space:pre-wrap;word-break:break-all}
.opts{margin:.5rem 0} input[type=number]{width:6rem}
</style></head><body>
<h1>ferryman — airgap 텍스트 전달용 QR</h1>
`

const encodeForm = `
<h2>① 텍스트 → QR</h2>
<form method="post" action="/encode">
<textarea name="text" placeholder="QR 로 만들 텍스트를 입력하세요">%s</textarea>
<div class="opts">
  <label>조각당 크기 <input type="number" name="chunk" value="%d" min="100" max="2900"></label>
  &nbsp; <label>모듈당 픽셀 <input type="number" name="scale" value="%d" min="2" max="16"></label>
</div>
<button type="submit">QR 생성</button>
</form>
`

const decodeForm = `
<h2>② 사진/이미지 → 텍스트</h2>
<form method="post" action="/decode" enctype="multipart/form-data">
<input type="file" name="images" accept="image/*" multiple>
<div><button type="submit">해독</button></div>
</form>
`

const pageBottom = `</body></html>`

// serve 는 웹 서버 모드를 실행한다.
func serve(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/encode", handleEncode)
	mux.HandleFunc("/decode", handleDecode)

	fmt.Printf("ferryman 웹 서버 실행: http://%s\n", addr)
	srv := &http.Server{Addr: addr, Handler: mux}
	return srv.ListenAndServe()
}

func writePage(w io.Writer, prefill string, body string) {
	io.WriteString(w, pageTop)
	fmt.Fprintf(w, encodeForm, html.EscapeString(prefill), defaultChunk, 6)
	io.WriteString(w, decodeForm)
	io.WriteString(w, body)
	io.WriteString(w, pageBottom)
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	writePage(w, "", "")
}

func handleEncode(w http.ResponseWriter, r *http.Request) {
	text := r.FormValue("text")
	chunk := atoiDefault(r.FormValue("chunk"), defaultChunk)
	scale := atoiDefault(r.FormValue("scale"), 6)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if text == "" {
		writePage(w, "", "<p>텍스트를 입력하세요.</p>")
		return
	}

	codes, err := encodeAll(text, chunk, recoveryLevel("l"))
	if err != nil {
		writePage(w, text, "<p>오류: "+html.EscapeString(err.Error())+"</p>")
		return
	}

	var body bytes.Buffer
	fmt.Fprintf(&body, "<h2>결과: QR %d개</h2><div class=\"grid\">", len(codes))
	for i, q := range codes {
		img, err := q.PNG(-scale) // 음수: 모듈당 픽셀 수
		if err != nil {
			continue
		}
		enc := base64.StdEncoding.EncodeToString(img)
		fmt.Fprintf(&body, "<div class=\"card\"><img src=\"data:image/png;base64,%s\"><span>%d / %d</span></div>", enc, i+1, len(codes))
	}
	body.WriteString("</div>")
	writePage(w, text, body.String())
}

func handleDecode(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writePage(w, "", "<p>업로드 오류: "+html.EscapeString(err.Error())+"</p>")
		return
	}
	files := r.MultipartForm.File["images"]
	if len(files) == 0 {
		writePage(w, "", "<p>이미지를 선택하세요.</p>")
		return
	}

	var texts []string
	var errs []string
	for _, fh := range files {
		f, err := fh.Open()
		if err != nil {
			errs = append(errs, fh.Filename+": "+err.Error())
			continue
		}
		t, err := decodeReader(f)
		f.Close()
		if err != nil {
			errs = append(errs, fh.Filename+": "+err.Error())
			continue
		}
		texts = append(texts, t)
	}

	body := "<h2>해독 결과</h2>"
	if result, err := reassemble(texts); err != nil {
		body += "<p>재조립 오류: " + html.EscapeString(err.Error()) + "</p>"
	} else if result != "" {
		body += "<pre>" + html.EscapeString(result) + "</pre>"
	}
	for _, e := range errs {
		body += "<p>⚠ " + html.EscapeString(e) + "</p>"
	}
	writePage(w, "", body)
}

func atoiDefault(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return n
	}
	return def
}
