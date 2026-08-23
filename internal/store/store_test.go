package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jbuitt/nwws-go-client/internal/product"
)

func testProduct() product.Product {
	return product.Product{
		CCCC:    "KKCI",
		TTAAII:  "FTUS21",
		AWIPSID: "TAFKORD",
		Issue:   time.Date(2026, 8, 22, 14, 32, 0, 0, time.UTC),
		ID:      "12345",
		Text:    "SAMPLE PRODUCT TEXT",
	}
}

func TestWriteProduct_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	p := testProduct()

	result, path, err := WriteProduct(dir, p)
	if err != nil {
		t.Fatalf("WriteProduct: %v", err)
	}
	if result != Written {
		t.Errorf("result = %v, want Written", result)
	}

	wantPath := filepath.Join(dir, "kkci", "kkci_ftus21-tafkord.221432_12345.txt")
	if path != wantPath {
		t.Errorf("path = %q, want %q", path, wantPath)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}
	if string(data) != p.Text {
		t.Errorf("file content = %q, want %q", string(data), p.Text)
	}
}

func TestWriteProduct_DuplicateIsSkippedNotOverwritten(t *testing.T) {
	dir := t.TempDir()
	p := testProduct()

	if _, _, err := WriteProduct(dir, p); err != nil {
		t.Fatalf("first WriteProduct: %v", err)
	}

	p2 := p
	p2.Text = "DIFFERENT TEXT THAT SHOULD NEVER BE WRITTEN"
	result, path, err := WriteProduct(dir, p2)
	if err != nil {
		t.Fatalf("second WriteProduct: %v", err)
	}
	if result != DuplicateSkipped {
		t.Errorf("result = %v, want DuplicateSkipped", result)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading file: %v", err)
	}
	if string(data) != p.Text {
		t.Errorf("file content = %q, want original %q (must not be overwritten)", string(data), p.Text)
	}
}

func TestWriteProduct_PermissionError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root, permission checks don't apply")
	}

	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	defer os.Chmod(dir, 0o700)

	_, _, err := WriteProduct(filepath.Join(dir, "readonly-parent"), testProduct())
	if err == nil {
		t.Fatal("WriteProduct: expected an error writing under a read-only directory")
	}
}
