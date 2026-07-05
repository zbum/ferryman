package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// recoveryLevel 은 문자 플래그(l/m/h/highest)를 라이브러리 레벨로 변환한다.
func recoveryLevel(s string) qrcode.RecoveryLevel {
	switch s {
	case "m", "medium":
		return qrcode.Medium
	case "h", "high":
		return qrcode.High
	case "x", "highest":
		return qrcode.Highest
	default:
		return qrcode.Low // 용량 우선(긴 텍스트/분할에 유리)
	}
}

// encodeAll 은 원문을 필요한 만큼 분할해 QR 코드 객체들로 만든다.
// bytesPerQR 는 QR 1개당 원문 바이트 수(사용자 조절값)다.
func encodeAll(content string, bytesPerQR int, level qrcode.RecoveryLevel) ([]*qrcode.QRCode, error) {
	payloads := splitPayloads(content, bytesPerQR)
	codes := make([]*qrcode.QRCode, 0, len(payloads))
	for _, p := range payloads {
		q, err := qrcode.New(p, level)
		if err != nil {
			// 대개 조각 하나가 QR 용량을 넘긴 경우 — 값을 줄이도록 안내한다.
			return nil, fmt.Errorf("QR 용량 초과: -bytes 값을 줄이세요(권장 100~%d): %w", maxBytesPerQR, err)
		}
		codes = append(codes, q)
	}
	return codes, nil
}

// writePNGs 는 QR 들을 PNG 파일로 저장하고 파일명 목록을 반환한다.
// 조각이 여러 개면 base 확장자 앞에 -1, -2 ... 를 붙인다.
func writePNGs(codes []*qrcode.QRCode, base string, scale int) ([]string, error) {
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	if ext == "" {
		ext = ".png"
	}

	names := make([]string, 0, len(codes))
	for i, q := range codes {
		name := base
		if len(codes) > 1 {
			name = fmt.Sprintf("%s-%d%s", stem, i+1, ext)
		}
		png, err := q.PNG(-scale) // 음수: 모듈당 픽셀 수
		if err != nil {
			return names, err
		}
		if err := os.WriteFile(name, png, 0o644); err != nil {
			return names, err
		}
		names = append(names, name)
	}
	return names, nil
}
