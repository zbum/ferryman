// ferryman 은 airgap(망분리) 환경에서 텍스트를 QR 사진으로 실어나르기 위한 도구다.
//
// 사용법:
//
//	ferryman [옵션] "텍스트"     터미널에 ANSI QR 출력 (텍스트 생략 시 stdin)
//	ferryman -i notes.md        파일(txt/md 등) 내용을 QR 로
//	ferryman -o out.png "텍스트"  PNG 파일로 저장 (분할 시 out-1.png ...)
//	ferryman serve -addr :8080   웹 서버 실행 (브라우저에서 생성/해독)
//	ferryman decode a.jpg b.jpg  사진에서 QR 을 읽어 원문 복원 (다중 자동 재조립)
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// version 은 빌드 시 -ldflags "-X main.version=..." 로 주입된다.
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "serve":
		serveCmd(os.Args[2:])
	case "decode":
		decodeCmd(os.Args[2:])
	case "version", "-v", "--version":
		fmt.Println("ferryman", version)
	case "-h", "--help", "help":
		printUsage()
	default:
		encodeCmd(os.Args[1:])
	}
}

func printUsage() {
	fmt.Fprint(os.Stderr, `ferryman — airgap 텍스트 전달용 QR 생성/해독기

사용법:
  ferryman [옵션] "텍스트"      터미널에 QR 출력 (텍스트 생략 시 stdin 에서 읽음)
  ferryman -i notes.md         파일(txt/md 등) 내용을 QR 로
  ferryman -o out.png "텍스트"   PNG 저장 (분할되면 out-1.png, out-2.png ...)
  ferryman serve [-addr :8080]  웹 서버 실행 (브라우저에서 생성/해독)
  ferryman decode a.jpg b.png   사진에서 QR 판독 후 원문 복원 (다중 자동 재조립)

인코딩 옵션:
  -i     입력 파일 경로 (txt/md 등). 지정 시 파일 내용을 인코딩
  -o     PNG 출력 파일명 (미지정 시 터미널 출력)
  -chunk 조각당 base64 문자 수 (기본 1000, 길면 여러 QR 로 분할)
  -level 오류정정 레벨 l|m|h|x (기본 l = 용량 우선)
  -scale PNG 모듈당 픽셀 (기본 8)
`)
}

// encodeCmd 는 텍스트를 QR 로 인코딩한다(터미널 또는 PNG).
// 입력 우선순위: -i 파일 > 위치 인자 텍스트 > stdin.
func encodeCmd(args []string) {
	fs := flag.NewFlagSet("encode", flag.ExitOnError)
	in := fs.String("i", "", "입력 파일 경로 (txt/md 등)")
	out := fs.String("o", "", "PNG 출력 파일명 (미지정 시 터미널 출력)")
	chunk := fs.Int("chunk", defaultChunk, "조각당 base64 문자 수")
	level := fs.String("level", "l", "오류정정 레벨 l|m|h|x")
	scale := fs.Int("scale", 8, "PNG 모듈당 픽셀 수")
	fs.Parse(args)

	text, err := readInput(*in, fs.Args())
	if err != nil {
		fatal(err)
	}
	if text == "" {
		fmt.Fprintln(os.Stderr, "입력 텍스트가 없습니다.")
		os.Exit(2)
	}

	codes, err := encodeAll(text, *chunk, recoveryLevel(*level))
	if err != nil {
		fatal(err)
	}

	if *out == "" {
		for i, q := range codes {
			if len(codes) > 1 {
				fmt.Printf("── QR %d / %d ──\n", i+1, len(codes))
			}
			renderTerminal(os.Stdout, q)
		}
		if len(codes) > 1 {
			fmt.Printf("\n총 %d개 QR. 순서대로 촬영하세요.\n", len(codes))
		}
		return
	}

	written, err := writePNGs(codes, *out, *scale)
	if err != nil {
		fatal(err)
	}
	for _, name := range written {
		fmt.Println("저장:", name)
	}
}

// readInput 은 -i 파일 / 위치 인자 / stdin 순으로 인코딩할 원문을 읽는다.
// 파일 내용은 바이트를 그대로 보존하고, 위치 인자·stdin 은 끝의 개행만 정리한다.
func readInput(inPath string, posArgs []string) (string, error) {
	if inPath != "" {
		data, err := os.ReadFile(inPath)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	if text := strings.Join(posArgs, " "); text != "" {
		return text, nil
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(data), "\n"), nil
}

// serveCmd 는 웹 서버를 실행한다.
func serveCmd(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", ":8080", "수신 주소")
	fs.Parse(args)
	if err := serve(*addr); err != nil {
		fatal(err)
	}
}

// decodeCmd 는 이미지들에서 QR 을 읽어 원문을 복원한다.
func decodeCmd(args []string) {
	fs := flag.NewFlagSet("decode", flag.ExitOnError)
	out := fs.String("o", "", "출력 텍스트 파일 (미지정 시 stdout)")
	fs.Parse(args)

	paths := fs.Args()
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "해독할 이미지 파일을 지정하세요.")
		os.Exit(2)
	}

	text, err := decodeFiles(paths)
	if err != nil {
		fatal(err)
	}

	if *out == "" {
		fmt.Print(text)
		if !strings.HasSuffix(text, "\n") {
			fmt.Println()
		}
		return
	}
	if err := os.WriteFile(*out, []byte(text), 0o644); err != nil {
		fatal(err)
	}
	fmt.Println("저장:", *out)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "오류:", err)
	os.Exit(1)
}
