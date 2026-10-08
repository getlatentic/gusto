package main

import (
	"archive/zip"
	"debug/pe"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// subsystemGUI is the value in a Windows program's header that says it has no console window of its own.
const subsystemGUI = 2

func TestTheWindowsHelperIsBuiltWithNoWindowOfItsOwn(t *testing.T) {
	if _, err := exec.LookPath("zip"); err != nil {
		t.Skip("no zip to pack with")
	}
	out := t.TempDir()
	command := exec.Command("sh", filepath.Join("..", "..", "deploy", "helper", "build.sh"), "dev", "windows", "amd64", out)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build.sh: %v\n%s", err, output)
	}
	archive, err := zip.OpenReader(filepath.Join(out, "haas-helper_dev_windows_amd64.zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	var program string
	for _, entry := range archive.File {
		file, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		program = filepath.Join(out, entry.Name)
		target, err := os.Create(program)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(target, file)
		target.Close()
		file.Close()
	}
	opened, err := pe.Open(program)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	header, ok := opened.OptionalHeader.(*pe.OptionalHeader64)
	if !ok || header.Subsystem != subsystemGUI {
		t.Errorf("the Windows helper's subsystem is %v, and %d is the one with no console window", opened.OptionalHeader, subsystemGUI)
	}
}
