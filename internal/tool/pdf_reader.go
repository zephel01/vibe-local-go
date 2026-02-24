package tool

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode/utf16"
)

// PDFTextExtractor extracts text from PDF files without external dependencies.
// This is a best-effort extractor that handles common PDF encodings.
type PDFTextExtractor struct{}

// NewPDFTextExtractor creates a new PDF text extractor
func NewPDFTextExtractor() *PDFTextExtractor {
	return &PDFTextExtractor{}
}

// ExtractText extracts text from a PDF file
func (e *PDFTextExtractor) ExtractText(path string, maxPages int) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("cannot read PDF file: %v", err)
	}

	// Verify PDF header
	if !bytes.HasPrefix(data, []byte("%PDF")) {
		return "", fmt.Errorf("not a valid PDF file")
	}

	// Extract text from all streams
	streams := extractStreams(data)
	if len(streams) == 0 {
		return "", fmt.Errorf("no readable content found in PDF")
	}

	var result strings.Builder
	pageCount := 0

	for _, stream := range streams {
		if maxPages > 0 && pageCount >= maxPages {
			result.WriteString(fmt.Sprintf("\n... (truncated at %d pages)\n", maxPages))
			break
		}

		text := extractTextFromStream(stream)
		if text != "" {
			if pageCount > 0 {
				result.WriteString("\n--- Page break ---\n\n")
			}
			result.WriteString(text)
			pageCount++
		}
	}

	if result.Len() == 0 {
		return "", fmt.Errorf("PDF contains no extractable text (may be image-based)")
	}

	return result.String(), nil
}

// extractStreams finds and decompresses all stream objects in the PDF
func extractStreams(data []byte) [][]byte {
	var streams [][]byte

	streamStart := regexp.MustCompile(`(?s)stream\r?\n`)
	streamEnd := []byte("endstream")

	matches := streamStart.FindAllIndex(data, -1)
	for _, match := range matches {
		start := match[1] // After "stream\n"
		endIdx := bytes.Index(data[start:], streamEnd)
		if endIdx < 0 {
			continue
		}

		raw := data[start : start+endIdx]

		// Try to find if this stream is FlateDecode (compressed)
		// Look backwards for the dictionary
		dictStart := findDictBefore(data, match[0])
		if dictStart >= 0 {
			dict := string(data[dictStart:match[0]])
			if strings.Contains(dict, "FlateDecode") {
				decompressed, err := decompressFlate(raw)
				if err == nil {
					streams = append(streams, decompressed)
					continue
				}
			}
		}

		// Use raw stream
		streams = append(streams, raw)
	}

	return streams
}

// findDictBefore finds the start of the dictionary before a stream
func findDictBefore(data []byte, streamPos int) int {
	// Look for "<<" before stream, within reasonable distance
	searchStart := streamPos - 2048
	if searchStart < 0 {
		searchStart = 0
	}

	chunk := data[searchStart:streamPos]
	// Find last "<<" in chunk
	idx := bytes.LastIndex(chunk, []byte("<<"))
	if idx >= 0 {
		return searchStart + idx
	}
	return -1
}

// decompressFlate decompresses FlateDecode data
func decompressFlate(data []byte) ([]byte, error) {
	reader, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// extractTextFromStream extracts readable text from a PDF content stream
func extractTextFromStream(stream []byte) string {
	var result strings.Builder
	content := string(stream)

	// Extract text from Tj (show string) and TJ (show array) operators
	tjPattern := regexp.MustCompile(`\(((?:[^()\\]|\\.|\\[0-7]{1,3})*)\)\s*Tj`)
	tjMatches := tjPattern.FindAllStringSubmatch(content, -1)
	for _, match := range tjMatches {
		text := decodePDFString(match[1])
		result.WriteString(text)
	}

	// Extract text from TJ arrays: [(text) kerning (text)] TJ
	tjArrayPattern := regexp.MustCompile(`\[((?:\([^)]*\)|[^]]*)*)\]\s*TJ`)
	tjArrayMatches := tjArrayPattern.FindAllStringSubmatch(content, -1)
	for _, match := range tjArrayMatches {
		text := extractFromTJArray(match[1])
		result.WriteString(text)
	}

	// Handle Td/TD (text positioning - approximate newlines)
	if strings.Contains(content, "Td") || strings.Contains(content, "TD") {
		// Basic newline detection from text positioning
		lines := tdNewlinePattern(content)
		if lines != "" && result.Len() == 0 {
			result.WriteString(lines)
		}
	}

	// Clean up result
	text := result.String()
	text = cleanPDFText(text)

	return text
}

// decodePDFString decodes a PDF string literal
func decodePDFString(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n':
				result.WriteByte('\n')
			case 'r':
				result.WriteByte('\r')
			case 't':
				result.WriteByte('\t')
			case '(':
				result.WriteByte('(')
			case ')':
				result.WriteByte(')')
			case '\\':
				result.WriteByte('\\')
			default:
				// Octal escape
				if s[i] >= '0' && s[i] <= '7' {
					octal := string(s[i])
					for j := 1; j < 3 && i+j < len(s) && s[i+j] >= '0' && s[i+j] <= '7'; j++ {
						octal += string(s[i+j])
					}
					var val byte
					for _, c := range octal {
						val = val*8 + byte(c-'0')
					}
					result.WriteByte(val)
					i += len(octal) - 1
				} else {
					result.WriteByte(s[i])
				}
			}
		} else {
			result.WriteByte(s[i])
		}
		i++
	}
	return result.String()
}

// extractFromTJArray extracts text from a TJ array string
func extractFromTJArray(s string) string {
	var result strings.Builder
	parenPattern := regexp.MustCompile(`\(([^)]*)\)`)
	matches := parenPattern.FindAllStringSubmatch(s, -1)
	for _, match := range matches {
		result.WriteString(decodePDFString(match[1]))
	}
	return result.String()
}

// tdNewlinePattern attempts to extract text with newline positioning from Td operators
func tdNewlinePattern(content string) string {
	// This is a simplified pattern — real PDF rendering is complex
	// Look for BT...ET blocks and extract text operations
	btPattern := regexp.MustCompile(`(?s)BT(.*?)ET`)
	btMatches := btPattern.FindAllStringSubmatch(content, -1)

	var result strings.Builder
	for _, match := range btMatches {
		block := match[1]
		// Extract positioned text
		lines := strings.Split(block, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasSuffix(line, "Tj") || strings.HasSuffix(line, "TJ") {
				// Already handled by Tj/TJ patterns
				continue
			}
			if strings.HasSuffix(line, "Td") || strings.HasSuffix(line, "TD") {
				result.WriteByte('\n')
			}
		}
	}
	return result.String()
}

// cleanPDFText cleans up extracted PDF text
func cleanPDFText(text string) string {
	// Replace multiple spaces with single space
	spacePattern := regexp.MustCompile(`[ \t]+`)
	text = spacePattern.ReplaceAllString(text, " ")

	// Replace multiple newlines with double newline
	nlPattern := regexp.MustCompile(`\n{3,}`)
	text = nlPattern.ReplaceAllString(text, "\n\n")

	// Trim
	text = strings.TrimSpace(text)

	return text
}

// decodeUTF16BE decodes UTF-16 BE bytes to string (for PDF Unicode strings)
func decodeUTF16BE(data []byte) string {
	if len(data) < 2 {
		return ""
	}

	// Check BOM
	start := 0
	if data[0] == 0xFE && data[1] == 0xFF {
		start = 2
	}

	// Decode UTF-16 BE
	u16s := make([]uint16, 0, (len(data)-start)/2)
	for i := start; i+1 < len(data); i += 2 {
		u16s = append(u16s, uint16(data[i])<<8|uint16(data[i+1]))
	}

	runes := utf16.Decode(u16s)
	return string(runes)
}
