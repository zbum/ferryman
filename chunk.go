package main

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// magic 는 다중 분할 QR 페이로드의 헤더 접두사다. 이 접두사로 시작하면
// 분할된 조각으로 취급하고, 아니면 단일 완결 메시지로 본다.
const magic = "QRG1"

// defaultBytesPerQR 는 QR 하나가 담는 원문 바이트 수의 기본값이다.
// 사용자가 조절 가능하다. 이 값이면 QR 이 충분히 낮은 버전이라 휴대폰 사진으로도
// 잘 스캔된다. 값을 키우면 장수는 줄지만 QR 이 조밀해져 스캔이 어려워진다.
const defaultBytesPerQR = 500

// maxBytesPerQR 는 QR 하나가 담을 수 있는 원문 바이트 상한이다. base64(약 4/3 확장)
// 를 거치면 이 값 부근에서 QR 버전이 높아져(≈v22 초과) 디코더/카메라가 검출에 실패하기
// 시작한다. 실측 기준 안전 상한으로 700 을 쓰고, 이를 넘는 입력은 잘라 안전을 보장한다.
const maxBytesPerQR = 700

// splitPayloads 는 원문을 QR 조각들의 페이로드 문자열 슬라이스로 나눈다.
// bytesPerQR 는 "QR 1개당 원문 바이트 수"로, 사용자가 조절한다.
//
//   - 한 QR 에 다 담기면 원문 그대로 1개 반환(일반 QR 앱에서 바로 읽힘).
//   - 여러 개가 필요하면 원문을 bytesPerQR 바이트씩 잘라 각 조각을 base64 로 감싸고
//     "QRG1|<id>|<seq>/<total>|<data>" 헤더를 붙인다. 임의 바이트/한글도 안전.
func splitPayloads(content string, bytesPerQR int) []string {
	if bytesPerQR <= 0 {
		bytesPerQR = defaultBytesPerQR
	}
	if bytesPerQR > maxBytesPerQR {
		bytesPerQR = maxBytesPerQR // 안전 상한 — 검출 실패 방지
	}
	data := []byte(content)
	// 단일 QR 로 충분하면 원문 그대로 — 표준 QR 리더로도 즉시 판독된다.
	if len(data) <= bytesPerQR {
		return []string{content}
	}

	sum := sha256.Sum256(data)
	id := fmt.Sprintf("%x", sum[:4]) // 8 hex chars, 그룹 식별자

	var chunks [][]byte
	for i := 0; i < len(data); i += bytesPerQR {
		end := i + bytesPerQR
		if end > len(data) {
			end = len(data)
		}
		chunks = append(chunks, data[i:end])
	}

	total := len(chunks)
	payloads := make([]string, total)
	for i, c := range chunks {
		enc := base64.StdEncoding.EncodeToString(c)
		payloads[i] = fmt.Sprintf("%s|%s|%d/%d|%s", magic, id, i+1, total, enc)
	}
	return payloads
}

// part 는 디코딩된 QR 조각 하나를 나타낸다.
type part struct {
	id    string
	seq   int
	total int
	data  string
}

// parsePart 는 QR 에서 읽은 텍스트가 분할 조각이면 (part, true) 를 반환한다.
// 헤더가 없으면 단일 완결 메시지이므로 false 를 반환한다.
func parsePart(text string) (part, bool) {
	if !strings.HasPrefix(text, magic+"|") {
		return part{}, false
	}
	fields := strings.SplitN(text, "|", 4)
	if len(fields) != 4 {
		return part{}, false
	}
	seqTotal := strings.SplitN(fields[2], "/", 2)
	if len(seqTotal) != 2 {
		return part{}, false
	}
	seq, err1 := strconv.Atoi(seqTotal[0])
	total, err2 := strconv.Atoi(seqTotal[1])
	if err1 != nil || err2 != nil {
		return part{}, false
	}
	return part{id: fields[1], seq: seq, total: total, data: fields[3]}, true
}

// reassemble 는 여러 QR 에서 읽은 텍스트들을 원문으로 복원한다.
//   - 분할 조각이 섞여 있으면 id 로 묶고, 각 조각을 base64 디코딩해 seq 순으로 이어 붙인다.
//   - 헤더 없는 단일 메시지는 그대로 반환한다.
func reassemble(texts []string) (string, error) {
	var singles []string
	groups := map[string][]part{}

	for _, t := range texts {
		if p, ok := parsePart(t); ok {
			groups[p.id] = append(groups[p.id], p)
		} else if t != "" {
			singles = append(singles, t)
		}
	}

	// 분할 그룹이 없으면 단일 메시지들을 이어 반환.
	if len(groups) == 0 {
		return strings.Join(singles, ""), nil
	}

	var out strings.Builder
	for id, parts := range groups {
		sort.Slice(parts, func(i, j int) bool { return parts[i].seq < parts[j].seq })

		total := parts[0].total
		seen := map[int][]byte{}
		for _, p := range parts {
			decoded, err := base64.StdEncoding.DecodeString(p.data)
			if err != nil {
				return "", fmt.Errorf("그룹 %s 조각 %d: base64 디코딩 실패: %w", id, p.seq, err)
			}
			seen[p.seq] = decoded
		}
		var missing []int
		for i := 1; i <= total; i++ {
			d, ok := seen[i]
			if !ok {
				missing = append(missing, i)
				continue
			}
			out.Write(d)
		}
		if len(missing) > 0 {
			return "", fmt.Errorf("그룹 %s: %d개 중 조각 %v 이(가) 없습니다", id, total, missing)
		}
	}
	// 단일 메시지도 있었다면 뒤에 덧붙인다(혼합 입력 대비).
	out.WriteString(strings.Join(singles, ""))
	return out.String(), nil
}
