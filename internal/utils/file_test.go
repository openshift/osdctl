package utils

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var (
	existingFolder    = "./testdata/a-folder-that-exists"
	nonExistingFolder = "./testdata/a-folder-that-does-not-exist"
	existingFile      = "./testdata/a-folder-that-exists/file.txt"
	nonExistingFile   = "./testdata/a-folder-that-exists/missing-file.txt"
	nonWritableDir    = "./testdata/non-writable-dir"
	fileToDelete      = "./testdata/a-folder-that-exists/a-file-to-be-deleted.txt"
	fileNotToDelete   = "./testdata/a-folder-that-exists/a-file-not-to-be-deleted.txt"
)

func ExampleFolderExists() {
	exists := FolderExists("/a-non-existing-folder")
	if exists {
		fmt.Println("Folder exists")
	} else {
		fmt.Println("Folder does not exist")
	}
	// Output: Folder does not exist
}

func TestFolderExists(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{
			"Returns true when given folder exists",
			existingFolder,
			true,
		},
		{
			"Returns false when given folder does not exist",
			nonExistingFolder,
			false,
		},
		{
			"Returns false when provided path is not a directory",
			existingFile,
			false,
		},
		{
			"Returns false when provided path is empty",
			"",
			false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if got := FolderExists(tt.path); got != tt.want {
				t.Errorf("FolderExists(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func ExampleFileExists() {
	exists := FileExists("/missing-file.txt")
	if exists {
		fmt.Println("File exists")
	} else {
		fmt.Println("File does not exist")
	}
	// Output: File does not exist
}

func TestFileExists(t *testing.T) {
	if err := os.Mkdir(nonWritableDir, 0400); err != nil {
		log.Fatalf("Cannot create non writable directory %q", nonWritableDir)
	}

	tests := []struct {
		name string
		path string
		want bool
	}{
		{
			"Returns true when given file exists",
			existingFile,
			true,
		},
		{
			"Returns false when given file does not exist",
			nonExistingFile,
			false,
		},
		{
			"Returns false when file is into a folder without read permissions",
			filepath.Join(nonWritableDir, "missing-file.txt"),
			false,
		},
		{
			"Returns false when provided path is a directory",
			existingFolder,
			false,
		},
		{
			"Returns false when provided path is empty",
			"",
			false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if got := FileExists(tt.path); got != tt.want {
				t.Errorf("FileExists(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}

	_ = os.RemoveAll(nonWritableDir)
}

func ExampleRemoveFile() {
	_ = CreateFile(fileToDelete)
	err := RemoveFile(fileToDelete)

	if err == nil {
		fmt.Println("File deleted")
	} else {
		fmt.Println("Unable to delete file")
	}
	// Output: File deleted
}

func TestRemoveFile(t *testing.T) {
	_ = CreateFile(fileToDelete)

	createImmutableFile(fileNotToDelete)

	defer deleteImmutableFile(fileNotToDelete)

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			"Returns nil if existing file is deleted",
			fileToDelete,
			false,
		},
		{
			"Returns err when asked to delete folder",
			existingFolder,
			true,
		},
		{
			"Returns err when asked to delete file that does not exist",
			nonExistingFile,
			true,
		},
		{
			"Returns err if asked to delete file that cannot be deleted ",
			fileNotToDelete,
			true,
		},
		{
			"Returns err if argument is empty path",
			"",
			true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			err := RemoveFile(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("RemoveFile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func ExampleCreateFile() {
	err := CreateFile(existingFile)
	if err != nil {
		fmt.Println("Failed to create file because it already exists")
	} else {
		fmt.Println("File has been created successfully!")
	}
	// Output: Failed to create file because it already exists
}

func TestCreateFile(t *testing.T) {
	if err := os.Mkdir(nonWritableDir, 0400); err != nil {
		log.Fatalf("Cannot create non writable directory %q", nonWritableDir)
	}

	defer func() {
		_ = os.Remove(nonExistingFile)
		_ = os.RemoveAll(nonWritableDir)
		_ = os.RemoveAll(nonExistingFolder)
	}()

	tests := []struct {
		name     string
		filepath string
		wantErr  bool
	}{
		{
			"Returns nil if file is created",
			nonExistingFile,
			false,
		},
		{
			"Returns err if file is exists",
			existingFile,
			true,
		},
		{
			"Returns nil if file is created along with parent dirs",
			filepath.Join(nonExistingFolder, "file.txt"),
			false,
		},
		{
			"Returns err if cannot write the file",
			filepath.Join(nonWritableDir, "should-not-write-this.txt"),
			true,
		},
		{
			"Returns err when provided filepath is empty",
			"",
			true,
		},
		{
			"Returns err if parent directory couldn't be created",
			filepath.Join(nonWritableDir, "parent/newdir/file.txt"),
			true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			skipWindowsNonWritableDirScenario(t, tt.filepath, tt.name)

			if err := CreateFile(tt.filepath); (err != nil) != tt.wantErr {
				t.Errorf("CreateFile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func skipWindowsNonWritableDirScenario(t *testing.T, file string, scenarioName string) {
	if strings.Contains(file, filepath.Base(nonWritableDir)) && runtime.GOOS == "windows" {
		t.Skipf("Skip %q test in windows", scenarioName)
	}
}

func createImmutableFile(file string) {
	_ = CreateFile(file)
	if err := os.Chmod(file, 0000); err != nil {
		log.Fatalf("Cannot create non writable file %q", nonWritableDir)
	}
}

func deleteImmutableFile(file string) {
	if err := os.Chmod(file, 0777); err != nil {
		log.Fatalf("Cannot change permissions to 0777 to file %q", nonWritableDir)
	}
	_ = os.Remove(file)
}
