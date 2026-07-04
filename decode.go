package main

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

// decodeReader 는 이미지 스트림 하나에서 QR 을 읽어 텍스트를 반환한다.
// 사진(JPEG)·스크린샷(PNG)·GIF 를 지원한다.
func decodeReader(r io.Reader) (string, error) {
	img, _, err := image.Decode(r)
	if err != nil {
		return "", fmt.Errorf("이미지 디코딩 실패: %w", err)
	}

	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return "", err
	}

	reader := qrcode.NewQRCodeReader()
	// TryHarder: 기울어지거나 흐릿한 사진에서 인식률을 높인다.
	hints := map[gozxing.DecodeHintType]interface{}{
		gozxing.DecodeHintType_TRY_HARDER: true,
	}
	result, err := reader.Decode(bmp, hints)
	if err != nil {
		return "", fmt.Errorf("QR 판독 실패: %w", err)
	}
	return result.GetText(), nil
}

// decodeImage 는 이미지 파일 하나에서 QR 을 읽어 텍스트를 반환한다.
func decodeImage(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	t, err := decodeReader(f)
	if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	return t, nil
}

// decodeFiles 는 여러 이미지에서 QR 을 읽어 원문으로 재조립한다.
func decodeFiles(paths []string) (string, error) {
	var texts []string
	for _, p := range paths {
		t, err := decodeImage(p)
		if err != nil {
			return "", err
		}
		texts = append(texts, t)
	}
	return reassemble(texts)
}
