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

// defaultChunk 는 조각 하나가 담는 base64 문자 수의 기본값이다.
// base64 는 QR byte 모드로 인코딩되며, L 레벨 기준 넉넉히 스캔 가능한 크기다.
const defaultChunk = 1000

// splitPayloads 는 원문을 QR 조각들의 페이로드 문자열 슬라이스로 나눈다.
//
//   - 한 조각에 들어가고 유효한 텍스트면 원문 그대로 1개 반환(일반 QR 앱에서 바로 읽힘).
//   - 여러 조각이 필요하면 전체를 base64 로 감싸 ASCII 조각으로 쪼갠 뒤
//     "QRG1|<id>|<seq>/<total>|<data>" 헤더를 붙인다. 임의 바이트/한글도 안전.
func splitPayloads(content string, chunkSize int) []string {
	if chunkSize <= 0 {
		chunkSize = defaultChunk
	}
	// 단일 QR 로 충분하면 원문 그대로 — 표준 QR 리더로도 즉시 판독된다.
	if len(content) <= chunkSize {
		return []string{content}
	}

	encoded := base64.StdEncoding.EncodeToString([]byte(content))
	sum := sha256.Sum256([]byte(content))
	id := fmt.Sprintf("%x", sum[:4]) // 8 hex chars, 그룹 식별자

	var pieces []string
	for i := 0; i < len(encoded); i += chunkSize {
		end := i + chunkSize
		if end > len(encoded) {
			end = len(encoded)
		}
		pieces = append(pieces, encoded[i:end])
	}

	total := len(pieces)
	payloads := make([]string, total)
	for i, p := range pieces {
		payloads[i] = fmt.Sprintf("%s|%s|%d/%d|%s", magic, id, i+1, total, p)
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
//   - 분할 조각이 섞여 있으면 id 로 묶고 seq 순으로 이어 붙여 base64 디코딩한다.
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
		seen := map[int]string{}
		for _, p := range parts {
			seen[p.seq] = p.data
		}
		var b64 strings.Builder
		var missing []int
		for i := 1; i <= total; i++ {
			d, ok := seen[i]
			if !ok {
				missing = append(missing, i)
				continue
			}
			b64.WriteString(d)
		}
		if len(missing) > 0 {
			return "", fmt.Errorf("그룹 %s: %d개 중 조각 %v 이(가) 없습니다", id, total, missing)
		}
		decoded, err := base64.StdEncoding.DecodeString(b64.String())
		if err != nil {
			return "", fmt.Errorf("그룹 %s: base64 디코딩 실패: %w", id, err)
		}
		out.Write(decoded)
	}
	// 단일 메시지도 있었다면 뒤에 덧붙인다(혼합 입력 대비).
	out.WriteString(strings.Join(singles, ""))
	return out.String(), nil
}
