package main

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func ByteCountSI(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
		fmt.Println(div, exp, unit)
	}
	return fmt.Sprintf("%.1f %cB",
		float64(b)/float64(div), "kMGTPE"[exp])
}

func Divisor(b int64) int64 {
	s := strings.Split(ByteCountSI(b), " ")
	var divisor int64

	switch s[1] {
	case "KB":
		divisor = 32768
	case "MB":
		divisor = 8388608
	case "GB":
		divisor = 104857600
	}
	return divisor
}

func checkForOldArchives(path string, patterns []string) ([]string, error) {
	var fileList []string
	path, err := getBasePath(path)
	if err != nil {
		return []string{}, fmt.Errorf("unable to test path : %v", err)
	}
	for i := range patterns {
		pattern := fmt.Sprintf("%v/*.%v", path, patterns[i])
		log.Printf("checking for patterns [%s]", pattern)
		files, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("unable to pattern match path, %v : %v", pattern, err)
		}
		fileList = append(fileList, files...)
	}
	return fileList, nil
}

func getBasePath(path string) (string, error) {
	filePath, err := os.Stat(path)
	// assume error is because the path includes the filename. try splitting off the filename
	if errors.Is(err, fs.ErrNotExist) {
		log.Printf("path is not found\nThis might be because the path includes a file\ntesting for a base path using [%v]", filepath.Dir(path))
		newFilePath, err := os.Stat(filepath.Dir(path))
		if err != nil {
			return "", err
		}
		// need to check if the resulting path is valid
		if newFilePath.IsDir() {
			return filepath.Dir(path), nil
		}
	}
	// if the error is not fs.ErrNotExist, assume it cannot be handled
	if err != nil {
		return "", err
	}
	// we should get here is path is a directory
	if filePath.IsDir() {
		return path, nil
	}
	//  something has gone horribly wrong!
	return "", fmt.Errorf("unknown error with path [%s]", path)
}

func deleteOldArchives(path string, patterns []string) error {
	fileList, err := checkForOldArchives(path, patterns)
	if err != nil {
		return err
	}
	for i := range fileList {
		f := fileList[i]
		log.Printf("found old archive [%s] deleting", f)
		err = os.Remove(f)
		if err != nil {
			return fmt.Errorf("err deleting file %s : %w", f, err)
		}
	}
	return nil
}
