package main

import (
	"encoding/base64"
	"strings"
	"testing"
)

// TestRoundTrip 은 splitPayloads → reassemble 가 원문을 완전 복원하는지 검증한다.
func TestRoundTrip(t *testing.T) {
	cases := []struct {
		name    string
		content string
		chunk   int
	}{
		{"짧은-ascii", "https://example.com/airgap", 1000},
		{"짧은-한글", "안녕하세요 airgap 테스트입니다", 1000},
		{"긴-한글-분할", strings.Repeat("가나다ABC123-", 250), 200},
		{"경계값-정확히한조각", strings.Repeat("x", 50), 50},
		{"경계값-한조각+1", strings.Repeat("y", 51), 50},
		{"이모지", strings.Repeat("🔒보안", 100), 120},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payloads := splitPayloads(tc.content, tc.chunk)
			if len(payloads) == 0 {
				t.Fatal("payload 가 비어 있음")
			}
			got, err := reassemble(payloads)
			if err != nil {
				t.Fatalf("reassemble 실패: %v", err)
			}
			if got != tc.content {
				t.Fatalf("원문 불일치\n want=%q\n got =%q", tc.content, got)
			}
		})
	}
}

// TestSinglePayloadIsRaw 는 단일 QR 이 헤더 없이 원문 그대로 나오는지 확인한다.
func TestSinglePayloadIsRaw(t *testing.T) {
	content := "hello"
	payloads := splitPayloads(content, 1000)
	if len(payloads) != 1 {
		t.Fatalf("조각 수 = %d, want 1", len(payloads))
	}
	if payloads[0] != content {
		t.Fatalf("단일 QR 이 원문과 다름: %q", payloads[0])
	}
}

// TestBytesPerQRClamped 는 상한을 넘는 값이 maxBytesPerQR 로 잘려
// 각 조각의 원문이 상한 이하로 유지되는지 확인한다.
func TestBytesPerQRClamped(t *testing.T) {
	content := strings.Repeat("가", 1000) // 3000 바이트
	// 상한(700)을 크게 넘는 값을 요청해도 조각당 원문은 700 이하여야 한다.
	payloads := splitPayloads(content, 5000)
	if len(payloads) < 2 {
		t.Fatalf("클램프 후에도 분할되어야 함: %d", len(payloads))
	}
	for i, p := range payloads {
		part, ok := parsePart(p)
		if !ok {
			t.Fatalf("조각 %d 헤더 파싱 실패", i)
		}
		raw, err := base64.StdEncoding.DecodeString(part.data)
		if err != nil {
			t.Fatalf("조각 %d base64 실패: %v", i, err)
		}
		if len(raw) > maxBytesPerQR {
			t.Fatalf("조각 %d 원문 %d바이트 > 상한 %d", i, len(raw), maxBytesPerQR)
		}
	}
	// 클램프해도 왕복은 정확해야 한다.
	got, err := reassemble(payloads)
	if err != nil {
		t.Fatalf("reassemble 실패: %v", err)
	}
	if got != content {
		t.Fatal("클램프 후 원문 불일치")
	}
}

// TestMissingPartFails 는 조각이 빠지면 오류를 내는지 확인한다.
func TestMissingPartFails(t *testing.T) {
	payloads := splitPayloads(strings.Repeat("z", 500), 100)
	if len(payloads) < 3 {
		t.Fatalf("분할이 충분하지 않음: %d", len(payloads))
	}
	// 두 번째 조각을 제거
	partial := append([]string{payloads[0]}, payloads[2:]...)
	if _, err := reassemble(partial); err == nil {
		t.Fatal("누락된 조각에도 오류가 없음")
	}
}

// TestUnorderedReassembly 는 조각 순서가 뒤섞여도 복원되는지 확인한다.
func TestUnorderedReassembly(t *testing.T) {
	content := strings.Repeat("정렬테스트-", 200)
	payloads := splitPayloads(content, 150)
	// 역순으로 뒤집기
	reversed := make([]string, len(payloads))
	for i := range payloads {
		reversed[len(payloads)-1-i] = payloads[i]
	}
	got, err := reassemble(reversed)
	if err != nil {
		t.Fatalf("reassemble 실패: %v", err)
	}
	if got != content {
		t.Fatal("뒤섞인 순서에서 복원 실패")
	}
}
