package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/schollz/progressbar/v3"
)

// uploadBlob handles the upload of a single blob to Azure
func uploadBlob(client *azblob.Client, containerName string, file tarFileStruct) error {
	_, err := client.UploadStream(context.Background(), containerName, file.Name, file.Content, nil)
	return err
}

// RestoreFromTarFile restores blobs from a tar archive using parallel uploads
func (b *BlobArchiver) RestoreFromTarFile() error {
	log.Printf("Opening tarfile [%s]", b.TarFile())

	// Open tar file for reading
	tarFile, err := os.Open(b.TarFile())
	if err != nil {
		return fmt.Errorf("failed to open tar file: %w", err)
	}
	defer tarFile.Close()
	tarfileStats, err := tarFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to get status of tar file: %w", err)
	}
	tarfileSize := tarfileStats.Size()

	// Create Azure Blob Storage client
	client, err := azblob.NewClientFromConnectionString(b.ConnectionString, nil)
	if err != nil {
		return fmt.Errorf("error creating Azure storage client: %w", err)
	}
	log.Print("Azure storage client created")

	var tarReader *tar.Reader
	var gzipReader *gzip.Reader

	if b.Compression {
		gzipReader, err = gzip.NewReader(tarFile)
		if err != nil {
			log.Fatal(err)
		}
		tarReader = tar.NewReader(gzipReader)
	} else {
		tarReader = tar.NewReader(tarFile)

	}
	numWorkers := b.Workers
	// Channel to send tar file contents to worker goroutines
	fileChan := make(chan tarFileStruct, numWorkers) // Buffered channel

	// WaitGroup to track worker progress
	var wg sync.WaitGroup

	// Worker pool for parallel uploads
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for file := range fileChan {
				err := uploadBlob(client, b.ContainerName, file)
				if err != nil {
					log.Printf("Failed to upload %s: %v", file.Name, err)
				}
			}
		}()
	}
	// testing bug where gzip file unzipped is larger than the tarFileSizeLimit
	if b.Compression {
		tarfileSize = -1
	}
	bar := progressbar.DefaultBytes(tarfileSize, "restoring tarfile")

	// Read tar file and send files to channel
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar file: %w", err)
		}

		blobName := header.Name
		if b.prefix != "" {
			blobName = fmt.Sprintf("%s/%s", b.prefix, blobName)
		}

		// Read the file content into a buffer
		var buf bytes.Buffer
		if _, err := io.Copy(io.MultiWriter(&buf, bar), tarReader); err != nil {
			return fmt.Errorf("failed to copy tar file content: %w", err)
		}

		// Send extracted file details to worker goroutines
		fileChan <- tarFileStruct{Name: blobName, Content: bytes.NewReader(buf.Bytes())}
	}

	// Close the channel to signal workers no more files will come
	close(fileChan)

	// Wait for all uploads to complete
	wg.Wait()

	log.Printf("Tar file restored to the source container %s", b.ContainerName)
	return nil
}
