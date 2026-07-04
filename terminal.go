package main

import (
	"bufio"
	"io"

	qrcode "github.com/skip2/go-qrcode"
)

// ansiDark 는 어두운 모듈(검정), ansiLight 는 밝은 모듈(흰색)의 truecolor 값이다.
const (
	ansiDark  = "0;0;0"
	ansiLight = "255;255;255"
)

// renderTerminal 은 QR 비트맵을 유니코드 반칸 문자(▀)로 그려 w 에 쓴다.
// 세로 픽셀 2줄을 한 줄에 담아 높이를 절반으로 줄이고, 위/아래 픽셀을
// 각각 전경/배경색으로 칠해 휴대폰 카메라로 바로 스캔 가능한 QR 을 만든다.
func renderTerminal(w io.Writer, q *qrcode.QRCode) {
	bw := bufio.NewWriter(w)
	defer bw.Flush()

	m := q.Bitmap() // [][]bool, true = 어두운 모듈. 여백(quiet zone) 포함.
	rows := len(m)
	for y := 0; y < rows; y += 2 {
		for x := 0; x < len(m[y]); x++ {
			top := color(m[y][x])
			bot := ansiLight
			if y+1 < rows {
				bot = color(m[y+1][x])
			}
			bw.WriteString("\x1b[38;2;")
			bw.WriteString(top)
			bw.WriteString("m\x1b[48;2;")
			bw.WriteString(bot)
			bw.WriteString("m▀") // ▀ upper half block
		}
		bw.WriteString("\x1b[0m\n")
	}
}

func color(dark bool) string {
	if dark {
		return ansiDark
	}
	return ansiLight
}
