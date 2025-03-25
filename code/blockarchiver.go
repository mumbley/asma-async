package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

// need a custom type for tags var
type StringMapFlag map[string]string

type BlobArchiver struct {
	ConnectionString            string
	ContainerName               string
	prefix                      string
	Compression                 bool
	Path                        string
	TarFileName                 string
	TarFileTags                 StringMapFlag
	Overwrite                   bool
	DestinationConnectionString string
	DestinationContainerName    string
	destinationPath             string
	TimeStr                     string
	BatchSize                   int
	Workers                     int
}

// NewBlobArchiver initializes a new BlobArchiver instance.
func NewBlobArchiver(
	connectionString string,
	containerName string,
	prefix string,
	compression bool,
	path,
	tarFileName string,
	tarFileTags map[string]string,
	overwrite bool,
	destinationConnectionString,
	destinationContainerName string,
	destinationPath string,
	TimeStr string,
	batchSize int,
	workers int,
) *BlobArchiver {
	return &BlobArchiver{
		ConnectionString:            connectionString,
		ContainerName:               containerName,
		prefix:                      prefix,
		Compression:                 compression,
		Path:                        path,
		TarFileName:                 tarFileName,
		TarFileTags:                 tarFileTags,
		Overwrite:                   overwrite,
		DestinationConnectionString: destinationConnectionString,
		DestinationContainerName:    destinationContainerName,
		destinationPath:             destinationPath,
		TimeStr:                     TimeStr,
		BatchSize:                   batchSize,
		Workers:                     workers,
	}
}

func (b BlobArchiver) destinationPrefix() string {
	if b.destinationPath == "" {
		return b.TimeStr
	}
	return fmt.Sprintf("%s/%s", b.TimeStr, b.destinationPath)
}

// TarFile computes the full path to the tar archive file.
func (b *BlobArchiver) TarFile() string {
	ext := "tar"
	if b.Compression {
		ext = "tgz"
	}

	if b.Path != "" && b.TarFileName != "" {
		return filepath.Join(b.Path, b.TarFileName)
	} else if b.TarFileName != "" {
		return b.TarFileName
	} else if b.Path != "" {
		return filepath.Join(b.Path, fmt.Sprintf("%s-%s.%s", b.ContainerName, b.TimeStr, ext))
	}
	return fmt.Sprintf("%s-%s.%s", b.ContainerName, b.TimeStr, ext)
}

type tarFileStruct struct {
	Name    string
	Content io.Reader
}

func (b *BlobArchiver) setDestinationTarFile() error {
	ext := "tar"
	if b.Compression {
		ext = "tgz"
	}
	info, err := os.Stat(b.destinationPath)
	if err != nil {
		return fmt.Errorf("unable to use path [%s] as tarfile destination", b.destinationPath)
	}
	if info.IsDir() {
		derivedPath := fmt.Sprintf("%s/%s-%s.%s", b.destinationPath, b.ContainerName, b.TimeStr, ext)
		log.Printf("destination path [%s] is a directory - using derived filename %s", b.destinationPath, derivedPath)
		b.destinationPath = derivedPath
	}
	return nil
}
