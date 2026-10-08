package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"runtime"
	"testing"
)

func TestAssetName(t *testing.T) {
	name, isZip := assetName()
	if runtime.GOOS == "windows" {
		if !isZip || name != fmt.Sprintf("envis-windows-%s.zip", runtime.GOARCH) {
			t.Fatalf("windows asset = %q isZip=%v", name, isZip)
		}
	} else {
		if isZip || name != fmt.Sprintf("envis-%s-%s.tar.gz", runtime.GOOS, runtime.GOARCH) {
			t.Fatalf("unix asset = %q isZip=%v", name, isZip)
		}
	}
}

func TestVerifyChecksum(t *testing.T) {
	archive := []byte("pretend archive bytes")
	sum := sha256.Sum256(archive)
	hexsum := hex.EncodeToString(sum[:])
	sums := []byte("deadbeef  other.tar.gz\n" + hexsum + "  envis-linux-amd64.tar.gz\n")

	if err := verifyChecksum(archive, "envis-linux-amd64.tar.gz", sums); err != nil {
		t.Fatalf("expected match, got %v", err)
	}
	// Wrong hash -> mismatch error.
	bad := []byte("0000  envis-linux-amd64.tar.gz\n")
	if err := verifyChecksum(archive, "envis-linux-amd64.tar.gz", bad); err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	// Missing entry -> error.
	if err := verifyChecksum(archive, "envis-linux-amd64.tar.gz", []byte("x  nope\n")); err == nil {
		t.Fatal("expected missing-entry error")
	}
}

func TestExtractBinaryTarGz(t *testing.T) {
	want := []byte("ELF-ish binary content")
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	_ = tw.WriteHeader(&tar.Header{Name: "envis", Mode: 0o755, Size: int64(len(want))})
	_, _ = tw.Write(want)
	tw.Close()
	gw.Close()

	got, err := extractBinary(buf.Bytes(), "envis", false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("tar.gz extract mismatch: got %q", got)
	}

	if _, err := extractBinary(buf.Bytes(), "missing", false); err == nil {
		t.Fatal("expected not-found error for missing binary")
	}
}

func TestExtractBinaryZip(t *testing.T) {
	want := []byte("PE-ish binary content")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("envis.exe")
	_, _ = w.Write(want)
	zw.Close()

	got, err := extractBinary(buf.Bytes(), "envis.exe", true)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("zip extract mismatch: got %q", got)
	}
}
