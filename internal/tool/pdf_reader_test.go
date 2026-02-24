package tool

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPDFTextExtractor_InvalidFile(t *testing.T) {
	extractor := NewPDFTextExtractor()

	// Non-existent file
	_, err := extractor.ExtractText("/nonexistent/file.pdf", 0)
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestPDFTextExtractor_NotPDF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fake.pdf")
	os.WriteFile(path, []byte("This is not a PDF"), 0644)

	extractor := NewPDFTextExtractor()
	_, err := extractor.ExtractText(path, 0)
	if err == nil {
		t.Error("expected error for non-PDF file")
	}
}

func TestPDFTextExtractor_MinimalPDF(t *testing.T) {
	// Create a minimal PDF with uncompressed text stream
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pdf")

	// This is a minimal valid PDF with a text stream
	pdf := []byte(`%PDF-1.4
1 0 obj
<< /Type /Catalog /Pages 2 0 R >>
endobj
2 0 obj
<< /Type /Pages /Kids [3 0 R] /Count 1 >>
endobj
3 0 obj
<< /Type /Page /Parent 2 0 R /Contents 4 0 R >>
endobj
4 0 obj
<< /Length 44 >>
stream
BT
/F1 12 Tf
(Hello World) Tj
ET
endstream
endobj
xref
0 5
trailer
<< /Size 5 /Root 1 0 R >>
startxref
0
%%EOF`)

	os.WriteFile(path, pdf, 0644)

	extractor := NewPDFTextExtractor()
	text, err := extractor.ExtractText(path, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if text == "" {
		t.Error("expected non-empty text")
	}

	if !containsSubstring(text, "Hello World") {
		t.Errorf("expected text to contain 'Hello World', got: %s", text)
	}
}

func TestPDFTextExtractor_TJArray(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test_tj.pdf")

	pdf := []byte(`%PDF-1.4
1 0 obj
<< /Type /Catalog /Pages 2 0 R >>
endobj
2 0 obj
<< /Type /Pages /Kids [3 0 R] /Count 1 >>
endobj
3 0 obj
<< /Type /Page /Parent 2 0 R /Contents 4 0 R >>
endobj
4 0 obj
<< /Length 60 >>
stream
BT
/F1 12 Tf
[(H) 10 (ello) -20 ( ) 10 (PDF)] TJ
ET
endstream
endobj
xref
0 5
trailer
<< /Size 5 /Root 1 0 R >>
startxref
0
%%EOF`)

	os.WriteFile(path, pdf, 0644)

	extractor := NewPDFTextExtractor()
	text, err := extractor.ExtractText(path, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !containsSubstring(text, "Hello PDF") {
		t.Errorf("expected text to contain 'Hello PDF', got: %s", text)
	}
}

func TestDecodePDFString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello", "Hello"},
		{"Hello\\nWorld", "Hello\nWorld"},
		{"Test\\(paren\\)", "Test(paren)"},
		{"Tab\\there", "Tab\there"},
		{"Back\\\\slash", "Back\\slash"},
	}

	for _, tt := range tests {
		result := decodePDFString(tt.input)
		if result != tt.expected {
			t.Errorf("decodePDFString(%q): expected %q, got %q", tt.input, tt.expected, result)
		}
	}
}

func TestCleanPDFText(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello   world", "hello world"},
		{"line1\n\n\n\n\nline2", "line1\n\nline2"},
		{"  trimmed  ", "trimmed"},
	}

	for _, tt := range tests {
		result := cleanPDFText(tt.input)
		if result != tt.expected {
			t.Errorf("cleanPDFText(%q): expected %q, got %q", tt.input, tt.expected, result)
		}
	}
}

func TestDecodeUTF16BE(t *testing.T) {
	// UTF-16 BE with BOM: "Hi"
	data := []byte{0xFE, 0xFF, 0x00, 0x48, 0x00, 0x69}
	result := decodeUTF16BE(data)
	if result != "Hi" {
		t.Errorf("expected 'Hi', got '%s'", result)
	}

	// Empty
	result = decodeUTF16BE([]byte{})
	if result != "" {
		t.Errorf("expected empty string, got '%s'", result)
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && findSubstring(s, sub))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
